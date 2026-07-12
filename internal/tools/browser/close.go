//go:build browser

package browser

import (
	"context"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// CloseTool shuts down the session's browser instance.
func (m *Manager) CloseTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "foxxycode_browser_close",
			Description: "Close the interactive browser for this session, releasing the Chrome process. Call when you are done with browser work.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		RequiresPermission: false,
		Execute:            m.executeClose,
	}
}

func (m *Manager) executeClose(_ context.Context, _ string, env *tooling.Env) (string, error) {
	m.closeSession(sessionKey(env))
	return "browser closed", nil
}
