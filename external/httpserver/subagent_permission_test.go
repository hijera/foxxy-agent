//go:build http

package httpserver

// The detached-permission broker: a subagent whose parent turn has ended still
// gets its prompt in front of the user, on the background task row, and the
// answer travels back through the ordinary permission endpoint addressed to
// the child session.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func newDetachedPermissionServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root,
		&session.FileStore{Root: filepath.Join(root, "sessions")})
	srv := New(cfg, mgr, slog.Default(), root)
	t.Cleanup(srv.Drain)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func detachedRequest(childID, toolCallID string) agent.DetachedPermissionRequest {
	return agent.DetachedPermissionRequest{
		ParentSessionID: "sess_parent",
		ChildSessionID:  childID,
		TaskID:          "bg_detached",
		AgentName:       "reviewer",
		Params: acp.PermissionRequestParams{
			SessionID: childID,
			ToolCall: acp.PermissionToolCall{
				ToolCallID: toolCallID,
				Title:      "[subagent reviewer] Run: run_command",
				Status:     "pending",
			},
			Options: []acp.PermissionOption{
				{OptionID: "allow", Name: "Allow once", Kind: "allow_once"},
				{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
			},
		},
	}
}

func TestDetachedPermissionIsAnsweredThroughTheChildSession(t *testing.T) {
	srv, ts := newDetachedPermissionServer(t)
	const childID = "sub_detached_ok"
	const toolCallID = "call_detached"

	answered := make(chan *acp.PermissionResult, 1)
	go func() {
		res, err := srv.RequestDetachedPermission(context.Background(), detachedRequest(childID, toolCallID))
		if err != nil {
			t.Errorf("RequestDetachedPermission error = %v", err)
		}
		answered <- res
	}()

	// The prompt has to be visible before anyone can answer it.
	var pending *detachedPermissionDTO
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pending = pendingDetachedPermission(childID); pending != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pending == nil {
		t.Fatal("the prompt was never published")
	}
	if pending.AgentName != "reviewer" || pending.ToolCall.ToolCallID != toolCallID {
		t.Fatalf("published prompt = %+v", pending)
	}

	// It reaches the client on the task row of the run that is waiting.
	row := newBackgroundTaskRow(bgtask.Snapshot{
		ID:        "bg_detached",
		SessionID: "sess_parent",
		Kind:      bgtask.KindAgent,
		Status:    bgtask.StatusRunning,
		Agent:     &bgtask.AgentInfo{Name: "reviewer", SessionID: childID},
		StartedAt: time.Now(),
	}, time.Now())
	if row.PendingPermission == nil || row.PendingPermission.ToolCall.ToolCallID != toolCallID {
		t.Fatalf("task row carries no pending prompt: %+v", row.PendingPermission)
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"pending_permission"`) {
		t.Fatalf("row JSON has no pending_permission: %s", encoded)
	}

	// Answered against the child session - the one actually waiting - through
	// the endpoint every other permission uses.
	body := `{"toolCallId":"` + toolCallID + `","optionId":"allow"}`
	status, _ := httpJSON(t, ts, http.MethodPost, "/foxxycode/sessions/"+childID+"/permission", body, nil)
	if status != http.StatusNoContent {
		t.Fatalf("answer status = %d, want 204", status)
	}

	select {
	case res := <-answered:
		if res == nil || res.OptionID != "allow" {
			t.Fatalf("child received %+v, want allow", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the answer never reached the waiting child")
	}
	if pendingDetachedPermission(childID) != nil {
		t.Fatal("the prompt outlived its answer")
	}
}

// Once nobody is waiting, the same id falls through to the read-only guard
// that keeps a child transcript from being prompted.
func TestPermissionOnAChildWithNoPromptStillRefuses(t *testing.T) {
	_, ts := newDetachedPermissionServer(t)
	body := `{"toolCallId":"call_nobody","optionId":"allow"}`
	status, _ := httpJSON(t, ts, http.MethodPost, "/foxxycode/sessions/sub_not_waiting/permission", body, nil)
	if status == http.StatusNoContent {
		t.Fatal("an answer nobody was waiting for was accepted")
	}
}

// The run's context is the deadline: the pool's timeout, an explicit stop and
// Drain all cancel it, and the waiter must let go rather than hold the task.
func TestDetachedPermissionUnblocksWhenTheRunIsCancelled(t *testing.T) {
	srv, _ := newDetachedPermissionServer(t)
	const childID = "sub_detached_cancel"
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan *acp.PermissionResult, 1)
	go func() {
		res, _ := srv.RequestDetachedPermission(ctx, detachedRequest(childID, "call_cancel"))
		done <- res
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && pendingDetachedPermission(childID) == nil {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	select {
	case res := <-done:
		if res != nil {
			t.Fatalf("a cancelled wait returned %+v, want no answer", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the wait outlived its run context")
	}
	if pendingDetachedPermission(childID) != nil {
		t.Fatal("a cancelled prompt stayed published")
	}
}

// A child narrowed to bypass is not asked at all, exactly as on the live path.
func TestDetachedPermissionShortCircuitsUnderBypass(t *testing.T) {
	srv, _ := newDetachedPermissionServer(t)
	req := detachedRequest("sub_bypass", "call_bypass")
	req.Params.EffectivePermissionMode = config.PermModeBypass
	res, err := srv.RequestDetachedPermission(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.OptionID != "allow" {
		t.Fatalf("bypass result = %+v", res)
	}
	if pendingDetachedPermission("sub_bypass") != nil {
		t.Fatal("a bypassed call still published a prompt")
	}
}
