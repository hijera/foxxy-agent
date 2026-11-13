package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestRestrictedWorkspaceSearchFeature(t *testing.T) {
	var h *settingsHarness
	suite := godog.TestSuite{
		Name: "restricted-workspace-search",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^an agent home inside the session workspace$`, func() error {
				h = newSettingsHarnessWith(t, func(c *config.Config) {
					c.Paths.Home = filepath.Join(c.Paths.CWD, "agent-home")
				}, "fake/a")
				home := h.mgr.Cfg().Paths.Home
				if err := os.MkdirAll(home, 0o700); err != nil {
					return err
				}
				// Sort before the public note: a forbidden match must not use
				// the single result slot before it is filtered out.
				if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("api_key: needle секрет credential\n"), 0o600); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(h.mgr.SessionByID(h.sessionID).GetCWD(), "note.txt"), []byte("needle секрет public note\n"), 0o600)
			})
			sc.Step(`^a restricted turn searches for "([^"]+)" with path "([^"]*)" and limit (\d+)$`, func(pattern, path string, limit int) error {
				args, err := json.Marshal(map[string]any{"pattern": pattern, "path": path, "max_results": limit})
				if err != nil {
					return err
				}
				h.provider("a").steps = []scriptStep{
					toolStep(llm.ToolCall{ID: "search", Name: "grep", InputJSON: string(args)}),
					answerStep("done"),
				}
				_, err = h.mgr.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
					SessionID: h.sessionID,
					Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "search the workspace"}},
				}, &refusingSender{}, &session.PromptRunOpts{Restriction: &session.TurnRestriction{
					AllowedTools: []string{"grep"}, AskAlways: true, ConfineToWorkspace: true,
				}})
				return err
			})
			sc.Step(`^the search returns the workspace note without the agent credential$`, func() error {
				for _, m := range h.mgr.SessionByID(h.sessionID).GetMessages() {
					if m.Role != llm.RoleTool || m.ToolCallID != "search" {
						continue
					}
					if strings.Contains(m.Content, "credential") || !strings.Contains(m.Content, "public note") {
						return fmt.Errorf("restricted search leaked a credential or lost the public note: %s", m.Content)
					}
					return nil
				}
				return fmt.Errorf("search produced no tool result")
			})
			sc.Step(`^a restricted turn lists files matching "([^"]+)"$`, func(pattern string) error {
				args, err := json.Marshal(map[string]any{"pattern": pattern})
				if err != nil {
					return err
				}
				return runRestrictedToolStep(h, llm.ToolCall{ID: "list", Name: "glob", InputJSON: string(args)}, "glob")
			})
			sc.Step(`^the listing shows the workspace note without any agent-home path$`, func() error {
				return restrictedToolResultShowsOnly(h, "list", "note.txt", "agent-home")
			})
			sc.Step(`^a restricted turn prints the directory tree$`, func() error {
				return runRestrictedToolStep(h, llm.ToolCall{ID: "tree", Name: "print_tree", InputJSON: `{}`}, "print_tree")
			})
			sc.Step(`^the tree shows the workspace note without any agent-home path$`, func() error {
				return restrictedToolResultShowsOnly(h, "tree", "note.txt", "agent-home")
			})
		},
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../../features/restricted_workspace_search.feature"}, TestingT: t, Strict: true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("restricted workspace search feature failed")
	}
}

// runRestrictedToolStep runs one call of a reading tool inside a restricted,
// workspace-confined turn - the same restriction a non-admin messenger turn
// carries for the search tools.
func runRestrictedToolStep(h *settingsHarness, call llm.ToolCall, allowed string) error {
	h.provider("a").steps = []scriptStep{
		toolStep(call),
		answerStep("done"),
	}
	_, err := h.mgr.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
		SessionID: h.sessionID,
		Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "search the workspace"}},
	}, &refusingSender{}, &session.PromptRunOpts{Restriction: &session.TurnRestriction{
		AllowedTools: []string{allowed}, AskAlways: true, ConfineToWorkspace: true,
	}})
	return err
}

// restrictedToolResultShowsOnly asserts the named tool result shows the
// workspace note and never names a path inside the agent home.
func restrictedToolResultShowsOnly(h *settingsHarness, callID, want, forbidden string) error {
	for _, m := range h.mgr.SessionByID(h.sessionID).GetMessages() {
		if m.Role != llm.RoleTool || m.ToolCallID != callID {
			continue
		}
		if strings.Contains(m.Content, forbidden) || !strings.Contains(m.Content, want) {
			return fmt.Errorf("restricted %s result leaked the agent home or lost the note: %s", callID, m.Content)
		}
		return nil
	}
	return fmt.Errorf("restricted %s call produced no tool result", callID)
}
