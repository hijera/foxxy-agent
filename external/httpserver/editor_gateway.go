//go:build http

package httpserver

import (
	"context"

	"github.com/hijera/foxxycode-agent/external/gateway"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/serve"
)

func (st *StartedHTTP) startEditorGateway() {
	if st.editor != "intellij" && st.editor != "vscode" {
		return
	}
	if st.gatewayCancel != nil {
		return
	}
	if !gateway.Available {
		st.Log.Warn("editor Telegram support requires -tags gateway")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	st.gatewayCancel = cancel
	st.gatewayDone = make(chan struct{})
	// Reuse the existing process runtime for chat wakes and detached permission
	// prompts, so the relay changes transport without removing gateway features.
	runtime := &serve.Runtime{Paths: st.Paths, Log: st.Log, Mgr: st.Manager}
	runtime.SetTurnMirror(st.Server)
	runtime.AddDetachedPermissionApprover(st.Server.detachedPromptBroker())
	st.Server.SetDetachedPrompts(runtime)
	runtime.AddWakeSurface(st.Server, agent.WakeHost)
	runtime.AttachBackgroundWaker()
	go func() {
		defer close(st.gatewayDone)
		if err := gateway.ServeEditor(ctx, st.editor, gateway.Options{
			Cfg: st.Config, Mgr: st.Manager, Log: st.Log,
			DefaultCWD: st.Paths.CWD, Mirror: st.Server, Prompts: runtime, Wakes: runtime,
		}); err != nil {
			st.Log.Warn("editor Telegram gateway", "error", err)
		}
	}()
}
