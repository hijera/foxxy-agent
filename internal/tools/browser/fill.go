//go:build browser

package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// FillTool sets the value of an input/textarea matched by a CSS selector.
func (m *Manager) FillTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "foxxycode_browser_fill",
			Description: "Type text into the input or textarea matching a CSS selector in the interactive browser (replaces the current value). Returns the resulting page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the input or textarea to fill.",
					},
					"text": map[string]interface{}{
						"type":        "string",
						"description": "Text to set as the field value.",
					},
				},
				"required": []string{"selector", "text"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeFill,
	}
}

type fillArgs struct {
	Selector string `json:"selector"`
	Text     string `json:"text"`
}

func (m *Manager) executeFill(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[fillArgs](argsJSON)
	if err != nil {
		return "", err
	}
	sel := strings.TrimSpace(args.Selector)
	if sel == "" {
		return "error: selector is required", nil
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	if err := b.run(
		chromedp.WaitVisible(sel, chromedp.ByQuery),
		chromedp.SetValue(sel, args.Text, chromedp.ByQuery),
	); err != nil {
		return fmt.Sprintf("error: fill %q: %v", sel, err), nil
	}
	return finishAction(b, env, fmt.Sprintf("filled %s", sel))
}
