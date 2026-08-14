package agent

import (
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// debugEnabled reports whether the diagnostics layer is active for this turn.
func (a *Agent) debugEnabled() bool {
	return a.cfg != nil && a.cfg.Debug.Enabled
}

// emitDebug records one structured debug-trace event (persisted as JSONL in the session
// bundle) and, when a sender is attached, forwards a lightweight DebugUpdate to ACP/SSE
// clients for a live debug view. It is a no-op unless the diagnostics layer is on and
// never affects the agent loop: tracing is best-effort.
func (a *Agent) emitDebug(turn int, phase, title, detail string, meta map[string]interface{}) {
	if !a.debugEnabled() {
		return
	}
	ev := session.DebugEvent{
		Turn:   turn,
		Phase:  phase,
		Title:  title,
		Detail: detail,
		At:     time.Now().UTC().Format(time.RFC3339),
		Meta:   meta,
	}
	if sd := a.state.GetPersistedSessionDir(); sd != "" {
		if _, err := session.AppendDebugEvent(sd, ev); err != nil && a.log != nil {
			a.log.Debug("debug trace append failed", "err", err)
		}
	}
	if a.server != nil {
		_ = a.server.SendSessionUpdate(a.state.GetID(), acp.DebugUpdate{
			SessionUpdate: acp.UpdateTypeDebug,
			Phase:         phase,
			Title:         title,
			Detail:        detail,
			Meta:          meta,
		})
	}
}
