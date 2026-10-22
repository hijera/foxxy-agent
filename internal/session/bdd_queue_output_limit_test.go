package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type queueOutputLimitScenario struct {
	turnEndState
	mgr      *session.Manager
	sid      string
	queueErr chan error
}

func (s *queueOutputLimitScenario) modelReachesLimit() error {
	if err := s.reset(); err != nil {
		return err
	}
	s.queueErr = make(chan error, 4)
	s.serve(func(n int, w io.Writer) {
		if n == 1 {
			for _, q := range []struct {
				text string
				mode session.QueueMode
			}{{"FIRST_DEFERRED", session.QueueModeAfterTurn}, {"UNREAD_STEER_FIRST", session.QueueModeSteer}, {"SECOND_DEFERRED", session.QueueModeAfterTurn}, {"UNREAD_STEER_SECOND", session.QueueModeSteer}} {
				_, _, err := s.mgr.EnqueueTurnMessageWithMode(s.sid, q.text, q.mode, nil)
				if err != nil {
					s.queueErr <- err
				}
			}
			sseChunk(w, map[string]any{"role": "assistant", "content": "PARTIAL_OUTPUT_4096"}, nil)
			sseChunk(w, map[string]any{}, "length")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		sseAnswer(w, fmt.Sprintf("DEFERRED_ANSWER_%d", n))
	})
	if err := s.agentWithoutLimit(); err != nil {
		return err
	}
	s.cfg.Models[0].MaxTokens = 4096
	off := false
	s.cfg.Title.Enabled = &off
	return nil
}

func (s *queueOutputLimitScenario) run() error {
	log := slog.Default()
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, snd acp.UpdateSender) (string, error) {
		s.state = st
		return agent.NewAgent(s.cfg, st, snd, log).Run(ctx, prompt)
	}
	s.mgr = session.NewManager(s.cfg, noopSender{}, runner, log, s.cwd, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	created, err := s.mgr.HandleSessionNew(ctx, acp.SessionNewParams{CWD: s.cwd})
	if err != nil {
		return err
	}
	s.sid = created.SessionID
	s.res, s.runErr = s.mgr.HandleSessionPrompt(ctx, acp.SessionPromptParams{SessionID: s.sid, Prompt: []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "INITIAL_PROMPT"}}})
	select {
	case err := <-s.queueErr:
		return err
	default:
		return s.runErr
	}
}

func (s *queueOutputLimitScenario) queuedPromptsRun() error {
	calls := s.stub.calls()
	if len(calls) != 4 || s.res.StopReason != acp.StopReasonEndTurn || s.res.StopNotice != "" {
		return fmt.Errorf("calls=%d, result=%+v; want the capped call, steering and two deferred answers", len(calls), s.res)
	}
	for i, want := range []string{"UNREAD_STEER_FIRST", "FIRST_DEFERRED", "SECOND_DEFERRED"} {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.Unmarshal(calls[i+1], &req); err != nil {
			return err
		}
		var lastUser string
		for _, m := range req.Messages {
			if m.Role == "user" && !strings.Contains(m.Content, "<turn_context>") {
				lastUser = m.Content
			}
		}
		if i == 0 {
			first, second := strings.Index(lastUser, "UNREAD_STEER_FIRST"), strings.Index(lastUser, "UNREAD_STEER_SECOND")
			if first < 0 || second <= first {
				return fmt.Errorf("steering batch=%q, want both corrections in their original order", lastUser)
			}
		} else if lastUser != want {
			return fmt.Errorf("request %d user=%q, want %q", i+2, lastUser, want)
		}
	}
	return nil
}

func (s *queueOutputLimitScenario) queuedMessagesOnce() error {
	var users strings.Builder
	for _, msg := range s.state.GetMessages() {
		if msg.Role == llm.RoleUser {
			users.WriteString(msg.Content)
		}
	}
	for _, want := range []string{"UNREAD_STEER_FIRST", "UNREAD_STEER_SECOND", "FIRST_DEFERRED", "SECOND_DEFERRED"} {
		if count := strings.Count(users.String(), want); count != 1 {
			return fmt.Errorf("%q occurs %d times in user messages, want exactly once", want, count)
		}
	}
	return nil
}

func (s *queueOutputLimitScenario) transcriptKeepsLimit() error {
	var transcript strings.Builder
	for _, msg := range s.state.GetMessages() {
		transcript.WriteString(msg.Content)
	}
	for _, want := range []string{"PARTIAL_OUTPUT_4096", "output limit", "4096", "FIRST_DEFERRED", "SECOND_DEFERRED"} {
		if !strings.Contains(transcript.String(), want) {
			return fmt.Errorf("transcript is missing %q", want)
		}
	}
	return nil
}

func (s *queueOutputLimitScenario) queueEmpty() error {
	rows, err := s.mgr.QueuedTurnMessages(s.sid)
	if err != nil {
		return err
	}
	if len(rows) != 0 {
		return fmt.Errorf("queue still contains %+v", rows)
	}
	return nil
}

func TestMessageQueueOutputLimitFeature(t *testing.T) {
	s := &queueOutputLimitScenario{}
	t.Cleanup(s.close)
	suite := godog.TestSuite{Name: "message_queue_output_limit", ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^a model that reaches its 4096 token output limit with both queue modes waiting$`, s.modelReachesLimit)
		sc.Step(`^the user starts that output-limited turn$`, s.run)
		sc.Step(`^the unread steering runs before the two deferred prompts$`, s.queuedPromptsRun)
		sc.Step(`^each queued message appears exactly once in the transcript$`, s.queuedMessagesOnce)
		sc.Step(`^the truncated answer and its limit notice remain in the transcript$`, s.transcriptKeepsLimit)
		sc.Step(`^no queued messages remain after the deferred answers$`, s.queueEmpty)
	}, Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/message_queue_output_limit.feature"}, TestingT: t, Strict: true}}
	if suite.Run() != 0 {
		t.Fatal("output-limit message queue feature failed")
	}
}
