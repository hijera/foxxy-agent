//go:build gateway || gateway.telegram

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hijera/foxxycode-agent/external/gateway/telegram"
	"github.com/hijera/foxxycode-agent/external/gateway/telegram/relay"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/logger"
)

// ServeEditor runs the project's adapter through a user-wide relay. Configuration
// replacements restart only this project's adapter, including enable/token edits.
func ServeEditor(ctx context.Context, editor string, opts Options) error {
	if editor != "intellij" && editor != "vscode" {
		return fmt.Errorf("unknown editor %q", editor)
	}
	if opts.Cfg == nil || opts.Mgr == nil || opts.Log == nil {
		return fmt.Errorf("editor gateway: Cfg, Mgr and Log are required")
	}
	changes := make(chan struct{}, 1)
	remove := opts.Mgr.AddConfigObserver(func(_ *config.Config) {
		select {
		case changes <- struct{}{}:
		default:
		}
	})
	defer remove()
	for ctx.Err() == nil {
		cfg := opts.Mgr.Cfg()
		snapshot, _ := json.Marshal(cfg.Gateways.Telegram)
		child, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- serveEditorBot(child, editor, cfg, opts) }()
		restart := false
		for !restart {
			select {
			case <-ctx.Done():
				cancel()
				if err := <-done; err != nil {
					opts.Log.Debug("editor Telegram gateway stopped", "error", err)
				}
				return nil
			case err := <-done:
				if err != nil {
					opts.Log.Warn("editor Telegram gateway stopped; retrying", "error", err)
				}
				select {
				case <-ctx.Done():
					cancel()
					return nil
				case <-changes:
				case <-time.After(3 * time.Second):
				}
				restart = true
			case <-changes:
				next, _ := json.Marshal(opts.Mgr.Cfg().Gateways.Telegram)
				if string(next) != string(snapshot) {
					cancel()
					if err := <-done; err != nil {
						opts.Log.Debug("editor Telegram gateway stopped", "error", err)
					}
					restart = true
				}
			}
		}
		cancel()
	}
	return nil
}

func serveEditorBot(ctx context.Context, editor string, cfg *config.Config, opts Options) error {
	tg := cfg.Gateways.Telegram
	if !tg.Enabled {
		<-ctx.Done()
		return nil
	}
	if tg.EffectiveToken() == "" {
		return fmt.Errorf("telegram is enabled but no bot token is configured")
	}
	// Resolve here: another editor may have launched the relay with a different
	// environment, and must not supply its TELEGRAM_BOT_TOKEN for this project.
	tg.Token = tg.EffectiveToken()
	project := relay.ProjectID(cfg.Paths.Home, opts.DefaultCWD)
	transport, err := relay.Connect(ctx, relay.Registration{
		Project: project, Label: opts.DefaultCWD, Editor: editor,
		APIBase: os.Getenv(config.TelegramAPIBaseEnv), Config: tg,
	})
	if err != nil {
		return err
	}
	defer transport.Close()
	storePath := filepath.Join(cfg.ResolvedSessionsRoot(), "gateway_editor_"+project+".json")
	bot := telegram.New(&tg, editorRunner{SessionRunner: opts.Mgr, cwd: opts.DefaultCWD}, opts.DefaultCWD,
		logger.Component(opts.Log, logger.ComponentGatewayTelegram), storePath, opts.Mirror)
	bot.SetHTTPClient(transport)
	if opts.Prompts != nil {
		bot.SetPromptSurfaces(opts.Prompts)
	}
	if opts.Wakes != nil {
		bot.SetWakeSurfaces(opts.Wakes)
	}
	return bot.Start(ctx)
}

// A relay selection also scopes /resume: a project's bot must not switch its
// conversation into a bundle whose working directory belongs to another project.
type editorRunner struct {
	telegram.SessionRunner
	cwd string
}

func (r editorRunner) HandleSessionList(ctx context.Context, params acp.SessionListParams) (*acp.SessionListResult, error) {
	params.CWD = &r.cwd
	return r.SessionRunner.HandleSessionList(ctx, params)
}
