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

// ClickTool clicks the first element matching a CSS selector, or a viewport
// point when only coordinates are given (for canvases, maps and widgets with no
// DOM handle to target).
func (m *Manager) ClickTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_click",
			Description: "Click in the interactive browser: the first element matching a CSS selector, or — when selector is omitted — " +
				"the viewport point (x, y), for canvases and widgets without a targetable element. Returns the resulting page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the element to click (e.g. \"#submit\", \"button.primary\"). Takes precedence over x/y.",
					},
					"x": map[string]interface{}{
						"type":        "number",
						"description": "Viewport x coordinate in pixels (used with y when selector is empty).",
					},
					"y": map[string]interface{}{
						"type":        "number",
						"description": "Viewport y coordinate in pixels (used with x when selector is empty).",
					},
				},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeClick,
	}
}

type clickArgs struct {
	Selector string   `json:"selector"`
	X        *float64 `json:"x"`
	Y        *float64 `json:"y"`
}

func (m *Manager) executeClick(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[clickArgs](argsJSON)
	if err != nil {
		return "", err
	}
	sel := strings.TrimSpace(args.Selector)
	if sel == "" && (args.X == nil || args.Y == nil) {
		return "error: selector (or both x and y) is required", nil
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	if sel != "" {
		if err := b.run(chromedp.Click(sel, chromedp.ByQuery, chromedp.NodeVisible)); err != nil {
			return fmt.Sprintf("error: click %q: %v", sel, err), nil
		}
		return finishAction(b, env, "clicked "+sel)
	}
	if err := b.run(chromedp.MouseClickXY(*args.X, *args.Y)); err != nil {
		return fmt.Sprintf("error: click at (%.0f,%.0f): %v", *args.X, *args.Y, err), nil
	}
	return finishAction(b, env, fmt.Sprintf("clicked at (%.0f,%.0f)", *args.X, *args.Y))
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
