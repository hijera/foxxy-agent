package agent

import (
	"context"
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
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

type stopPermissionSender struct {
	resumePermissionSender
	cancel context.CancelFunc
}

func (s *stopPermissionSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	s.cancel()
	return &acp.PermissionResult{Outcome: "cancelled", OptionID: "reject"}, nil
}

func TestPermissionStopFeature(t *testing.T) {
	var ag *Agent
	var st *session.State
	var sender *stopPermissionSender
	var provider *permTimeoutProvider
	var commandRan bool
	suite := godog.TestSuite{
		Name: "permission-stop",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^an agent whose command requires approval$`, func() error {
				dir := t.TempDir()
				configPath := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(configPath, []byte("tools:\n  permission_mode: ask\n"), 0o600); err != nil {
					return err
				}
				cfg, err := config.Load(configPath)
				if err != nil {
					return err
				}
				cfg.Providers = []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}}
				cfg.Models = []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}}
				cfg.Agent.Model = "fake/model"
				st = &session.State{ID: "sess_permission_stop", CWD: dir, SessionDir: dir, Mode: session.ModeAgent}
				sender = &stopPermissionSender{}
				provider = &permTimeoutProvider{}
				ag = NewAgent(cfg, st, sender, nil)
				command, ok := ag.registry.Get("run_command")
				if !ok {
					return fmt.Errorf("command tool missing")
				}
				command.Execute = func(context.Context, string, *tooling.Env) (string, error) {
					commandRan = true
					return "executed", nil
				}
				ag.SetConfigReloader(func(context.Context) ([]string, error) { return nil, nil })
				ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
				return nil
			})
			sc.Step(`^the operator stops the turn at the permission prompt$`, func() error {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				sender.cancel = cancel
				_, err := ag.Run(ctx, []acp.ContentBlock{{Type: "text", Text: "run a command"}})
				return err
			})
			sc.Step(`^the command has a persisted cancelled result$`, func() error {
				meta, err := session.ReadToolCallMeta(st.SessionDir, "perm-1")
				if err != nil {
					return err
				}
				if meta.Status != "cancelled" || meta.FinishedAt == "" {
					return fmt.Errorf("unfinished permission call: %+v", meta)
				}
				result, err := session.ReadToolCallResult(st.SessionDir, "perm-1")
				if err != nil {
					return err
				}
				if strings.TrimSpace(result) != permissionDeniedByUser {
					return fmt.Errorf("missing not-granted result: %q", result)
				}
				return nil
			})
			sc.Step(`^the command was never executed$`, func() error {
				if commandRan {
					return fmt.Errorf("unapproved command executed")
				}
				if provider.calls != 1 {
					return fmt.Errorf("model continued after Stop: %d calls", provider.calls)
				}
				return nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/permission_stop.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("permission Stop feature failed")
	}
}
