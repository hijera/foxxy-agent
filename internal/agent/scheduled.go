package agent

import (
	"context"
	"log/slog"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// RunScheduledTurn executes one agent turn over ephemeral session state (no disk session bundle).
// Returns the last assistant text block when available.
func RunScheduledTurn(ctx context.Context, cfg *config.Config, state SessionState, log *slog.Logger, snd acp.UpdateSender, instruction string) (assistantText string, stopReason string, err error) {
	a := NewAgent(cfg, state, snd, log)
	stopReason, err = a.Run(ctx, []acp.ContentBlock{{Type: acp.ContentTypeText, Text: instruction}})
	msgs := state.GetMessages()
	return lastAssistantPlainText(msgs), stopReason, err
}

func lastAssistantPlainText(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != llm.RoleAssistant {
			continue
		}
		if s := strings.TrimSpace(msgs[i].Content); s != "" {
			return msgs[i].Content
		}
	}
	return ""
}
