package todo

import (
	"github.com/hijera/foxxy-agent/internal/acp"
	"github.com/hijera/foxxy-agent/internal/tooling"
)

func sendPlanUpdate(env *tooling.Env, entries []acp.PlanEntry) {
	if env.Sender == nil {
		return
	}
	_ = env.Sender.SendSessionUpdate(env.SessionID, acp.PlanUpdate{
		SessionUpdate: acp.UpdateTypePlan,
		Entries:       entries,
	})
}
