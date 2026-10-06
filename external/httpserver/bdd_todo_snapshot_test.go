//go:build http

package httpserver

// Godog harness for features/todo_tool_card_snapshot.feature: the plan snapshot a
// todo tool call recorded must reach GET /foxxycode/sessions/{id}/tool-calls no
// matter in which order the call's meta.json was finished and snapshotted.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type todoSnapshotFeatureState struct {
	root       string
	ts         *httptest.Server
	srv        *Server
	mgr        *session.Manager
	sessionID  string
	sessionDir string
	toolCallID string
	plan       []acp.PlanEntry
	listed     []struct {
		ToolCallID   string          `json:"toolCallId"`
		Status       string          `json:"status"`
		PlanSnapshot []acp.PlanEntry `json:"planSnapshot"`
	}
}

func (s *todoSnapshotFeatureState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-todo-snapshot-*")
	if err != nil {
		return err
	}
	s.root = root
	s.sessionID = ""
	s.sessionDir = ""
	s.toolCallID = ""
	s.plan = nil
	s.listed = nil
	return nil
}

func (s *todoSnapshotFeatureState) close() {
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.srv != nil {
		s.srv.Drain()
		s.srv = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

func (s *todoSnapshotFeatureState) runningServer() error {
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{}
	store := &session.FileStore{Root: filepath.Join(s.root, "sessions")}
	cwd := filepath.Join(s.root, "work")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		return err
	}
	s.mgr = session.NewManager(cfg, noopSender{}, runner, slog.Default(), cwd, store)
	s.srv = New(cfg, s.mgr, slog.Default(), cwd)
	s.ts = httptest.NewServer(s.srv.Handler())
	return nil
}

func (s *todoSnapshotFeatureState) sessionWithTodoCall(toolCallID string) error {
	created, err := s.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: filepath.Join(s.root, "work")})
	if err != nil {
		return err
	}
	st := s.mgr.SessionByID(created.SessionID)
	if st == nil {
		return fmt.Errorf("session %s missing after creation", created.SessionID)
	}
	s.sessionID = created.SessionID
	s.sessionDir = st.GetPersistedSessionDir()
	s.toolCallID = toolCallID
	s.plan = []acp.PlanEntry{
		{Content: "Inspect cards", Status: "completed"},
		{Content: "Render preview", Status: "in_progress"},
	}
	st.AddMessage(llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{
			ID:        toolCallID,
			Name:      "foxxycode_todo_item_update",
			InputJSON: `{"index":1,"status":"in_progress"}`,
		}},
	})
	st.AddMessage(llm.Message{Role: llm.RoleTool, ToolCallID: toolCallID, Content: "updated item 1"})
	return session.MarkToolCallStarted(s.sessionDir, toolCallID, "foxxycode_todo_item_update", "todo", "in_progress")
}

func (s *todoSnapshotFeatureState) snapshotThenFinish() error {
	if err := session.WriteToolCallPlanSnapshot(s.sessionDir, s.toolCallID, s.plan); err != nil {
		return err
	}
	return session.MarkToolCallFinished(s.sessionDir, s.toolCallID, "foxxycode_todo_item_update", "todo", "completed")
}

func (s *todoSnapshotFeatureState) finishThenSnapshot() error {
	if err := session.MarkToolCallFinished(s.sessionDir, s.toolCallID, "foxxycode_todo_item_update", "todo", "completed"); err != nil {
		return err
	}
	return session.WriteToolCallPlanSnapshot(s.sessionDir, s.toolCallID, s.plan)
}

func (s *todoSnapshotFeatureState) listToolCalls() error {
	res, err := http.Get(s.ts.URL + "/foxxycode/sessions/" + s.sessionID + "/tool-calls")
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET tool-calls: status %d", res.StatusCode)
	}
	var body struct {
		ToolCalls []struct {
			ToolCallID   string          `json:"toolCallId"`
			Status       string          `json:"status"`
			PlanSnapshot []acp.PlanEntry `json:"planSnapshot"`
		} `json:"toolCalls"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	s.listed = body.ToolCalls
	return nil
}

func (s *todoSnapshotFeatureState) listedCallCarriesSnapshot(toolCallID string) error {
	for _, row := range s.listed {
		if row.ToolCallID != toolCallID {
			continue
		}
		if row.Status != "completed" {
			return fmt.Errorf("tool call %s status = %q, want completed", toolCallID, row.Status)
		}
		if len(row.PlanSnapshot) != len(s.plan) {
			return fmt.Errorf("tool call %s planSnapshot = %+v, want %+v", toolCallID, row.PlanSnapshot, s.plan)
		}
		for i := range s.plan {
			if row.PlanSnapshot[i] != s.plan[i] {
				return fmt.Errorf("tool call %s planSnapshot[%d] = %+v, want %+v", toolCallID, i, row.PlanSnapshot[i], s.plan[i])
			}
		}
		return nil
	}
	return fmt.Errorf("tool call %s not listed: %+v", toolCallID, s.listed)
}

func initializeTodoSnapshotScenario(sc *godog.ScenarioContext) {
	s := &todoSnapshotFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server$`, s.runningServer)
	sc.Step(`^a session with a todo tool call "([^"]+)" that updated a two-item plan$`, s.sessionWithTodoCall)
	sc.Step(`^the plan snapshot is saved and then the call is marked completed$`, s.snapshotThenFinish)
	sc.Step(`^the call is marked completed and then the plan snapshot is saved$`, s.finishThenSnapshot)
	sc.Step(`^the UI lists the session's tool calls$`, s.listToolCalls)
	sc.Step(`^the tool call "([^"]+)" is completed and carries the two-item plan snapshot$`, s.listedCallCarriesSnapshot)
}

func TestTodoToolCardSnapshotFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "todo-tool-card-snapshot",
		ScenarioInitializer: initializeTodoSnapshotScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/todo_tool_card_snapshot.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("todo tool card snapshot feature suite failed")
	}
}
