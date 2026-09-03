package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

// TestPermissionRequired pins the (permission mode, tool, session grants)
// decision matrix. The switch inside executeToolCall used to carry duplicate
// accept_edits/ask branches and a dead write-grant computation; this table is
// the behavioural contract that replacement must keep.
func TestPermissionRequired(t *testing.T) {
	cwd := t.TempDir()
	writeKey := "write|" + filepath.Join(cwd, "a.txt")
	toolsEnv := func(mode string, allowlist []string) *tools.Env {
		return &tools.Env{CWD: cwd, PermissionMode: mode, CommandAllowlist: allowlist}
	}
	runCmd := llm.ToolCall{Name: "run_command", InputJSON: `{"command":"go test ./..."}`}
	writeFile := llm.ToolCall{Name: "write", InputJSON: `{"path":"a.txt","content":"x"}`}
	configCommit := llm.ToolCall{Name: "config_commit", InputJSON: `{}`}

	tests := []struct {
		name              string
		registryRequires  bool
		tc                llm.ToolCall
		mode              string
		commandAllowlist  []string
		sessCmdGrants     []string
		sessWriteGrants   []string
		wantPermissionReq bool
	}{
		// run_command: bypass never prompts; every other mode consults the
		// session command grants (accept_edits behaves like ask by design).
		{"run_command bypass", true, runCmd, config.PermModeBypass, nil, nil, nil, false},
		{"run_command accept_edits ungranted", true, runCmd, config.PermModeAcceptEdits, nil, nil, nil, true},
		{"run_command accept_edits granted", true, runCmd, config.PermModeAcceptEdits, nil, []string{"go test ./..."}, nil, false},
		{"run_command ask ungranted", true, runCmd, config.PermModeAsk, nil, nil, nil, true},
		{"run_command ask granted", true, runCmd, config.PermModeAsk, nil, []string{"go test ./..."}, nil, false},
		{"run_command ask allowlisted", true, runCmd, config.PermModeAsk, []string{"go test"}, nil, nil, false},

		// Filesystem writes: accept_edits auto-approves without consulting
		// grants (the dead keys computation used to hide this); ask consults
		// the session write grants.
		{"write bypass", true, writeFile, config.PermModeBypass, nil, nil, nil, false},
		{"write accept_edits ungranted", true, writeFile, config.PermModeAcceptEdits, nil, nil, nil, false},
		{"write ask granted", true, writeFile, config.PermModeAsk, nil, nil, []string{writeKey}, false},
		{"write ask ungranted", true, writeFile, config.PermModeAsk, nil, nil, nil, true},

		// Config writes always prompt outside bypass: they can start MCP
		// processes and change the permission policy itself.
		{"config_commit bypass", true, configCommit, config.PermModeBypass, nil, nil, nil, false},
		{"config_commit accept_edits", true, configCommit, config.PermModeAcceptEdits, nil, nil, nil, true},
		{"config_commit ask", true, configCommit, config.PermModeAsk, nil, nil, nil, true},

		// Everything else follows the registry flag.
		{"registry flag false", false, llm.ToolCall{Name: "read", InputJSON: `{}`}, config.PermModeAsk, nil, nil, nil, false},
		{"registry flag true", true, llm.ToolCall{Name: "server__probe", InputJSON: `{}`}, config.PermModeAsk, nil, nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := permissionRequired(tt.registryRequires, tt.tc, toolsEnv(tt.mode, tt.commandAllowlist), tt.sessCmdGrants, tt.sessWriteGrants)
			if got != tt.wantPermissionReq {
				t.Fatalf("permissionRequired(%q, %q) = %v, want %v", tt.mode, tt.tc.Name, got, tt.wantPermissionReq)
			}
		})
	}
}

// hangingPermissionSender mimics a connected but unresponsive client: it
// sits on the permission request until the context dies, exactly like
// acp.Server.RequestPermission watching a client that never answers.
type hangingPermissionSender struct {
	deadlineSeen atomic.Bool
}

func (s *hangingPermissionSender) SendSessionUpdate(string, interface{}) error { return nil }

func (s *hangingPermissionSender) RequestPermission(ctx context.Context, _ acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	<-ctx.Done()
	if ctx.Err() == context.DeadlineExceeded {
		s.deadlineSeen.Store(true)
	}
	return &acp.PermissionResult{Outcome: "cancelled"}, nil
}

func (s *hangingPermissionSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

type permTimeoutProvider struct{ calls int }

func (p *permTimeoutProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *permTimeoutProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	if p.calls == 1 {
		call := llm.ToolCall{ID: "perm-1", Name: "run_command", InputJSON: `{"command":"echo unlisted"}`}
		onChunk(llm.StreamChunk{ToolCall: &call})
		return &llm.Response{ToolCalls: []llm.ToolCall{call}, StopReason: "tool_use"}, nil
	}
	return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
}

// TestPermissionPromptTimeoutCancelsToolCall pins tools.permission_timeout_seconds:
// a client that never answers the permission dialog must not hold the turn
// forever; after the configured deadline the tool call is cancelled and the
// turn finishes with the permission-denied marker.
func TestPermissionPromptTimeoutCancelsToolCall(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("tools:\n  permission_timeout_seconds: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers = []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}}
	cfg.Models = []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}}
	cfg.Agent.Model = "fake/model"
	st := &session.State{ID: "sess_perm_timeout", CWD: dir, Mode: session.ModeAgent}
	sender := &hangingPermissionSender{}
	provider := &permTimeoutProvider{}
	ag := NewAgent(cfg, st, sender, nil)
	ag.SetConfigReloader(func(context.Context) ([]string, error) { return nil, nil })
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }

	type runResult struct {
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		_, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "run something"}})
		done <- runResult{err: err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("Run: %v", res.err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run hung waiting for the permission prompt: the timeout did not fire")
	}

	if !sender.deadlineSeen.Load() {
		t.Fatal("permission context was not bounded by the configured deadline")
	}
	denied := false
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleTool && m.Content == "permission denied by user" {
			denied = true
		}
	}
	if !denied {
		t.Fatal("timed-out tool call did not leave the permission-denied marker")
	}
}
