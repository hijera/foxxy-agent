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

// ClickTool clicks the first element matching a CSS selector.
func (m *Manager) ClickTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "foxxycode_browser_click",
			Description: "Click the first element matching a CSS selector in the interactive browser. Returns the resulting page state and a screenshot.",
			InputSchema: selectorSchema("CSS selector of the element to click (e.g. \"#submit\", \"button.primary\")."),
		},
		RequiresPermission: true,
		Execute:            m.executeClick,
	}
}

func (m *Manager) executeClick(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[selectorArgs](argsJSON)
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
	if err := b.run(chromedp.Click(sel, chromedp.ByQuery, chromedp.NodeVisible)); err != nil {
		return fmt.Sprintf("error: click %q: %v", sel, err), nil
	}
	return finishAction(b, env, "clicked "+sel)
}

// selectorArgs is the common {selector} argument shape.
type selectorArgs struct {
	Selector string `json:"selector"`
}

func selectorSchema(desc string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"selector": map[string]interface{}{
				"type":        "string",
				"description": desc,
			},
		},
		"required": []string{"selector"},
	}
}
