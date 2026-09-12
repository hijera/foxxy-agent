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

// NavigateBackTool traverses the tab's history. Without it the only way back is
// re-navigating to a full URL the model may no longer have.
func (m *Manager) NavigateBackTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_navigate_back",
			Description: "Go back one entry in the interactive browser's history (or forward with direction=\"forward\"). " +
				"Returns the resulting page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"direction": map[string]interface{}{
						"type":        "string",
						"description": "History direction. Default \"back\".",
						"enum":        []interface{}{"back", "forward"},
					},
				},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeNavigateBack,
	}
}

type navigateBackArgs struct {
	Direction string `json:"direction"`
}

func (m *Manager) executeNavigateBack(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[navigateBackArgs](argsJSON)
	if err != nil {
		return "", err
	}
	dir := strings.ToLower(strings.TrimSpace(args.Direction))
	if dir == "" {
		dir = "back"
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	switch dir {
	case "back":
		if err := b.run(chromedp.NavigateBack()); err != nil {
			return fmt.Sprintf("error: navigate back: %v", err), nil
		}
		return finishAction(b, env, "navigated back")
	case "forward":
		if err := b.run(chromedp.NavigateForward()); err != nil {
			return fmt.Sprintf("error: navigate forward: %v", err), nil
		}
		return finishAction(b, env, "navigated forward")
	default:
		return fmt.Sprintf("error: direction must be back or forward (got %q)", args.Direction), nil
	}
}
