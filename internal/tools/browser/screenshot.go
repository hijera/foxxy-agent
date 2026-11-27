//go:build browser

package browser

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// ScreenshotTool captures the current page without performing any other action.
func (m *Manager) ScreenshotTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "foxxycode_browser_screenshot",
			Description: "Capture a screenshot of the current page in the interactive browser so you can see it. Use after actions when you need a fresh view of the page.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"screenshot_prompt": screenshotPromptProperty(),
				},
			},
		},
		RequiresPermission: false,
		Execute:            m.executeScreenshot,
	}
}

func (m *Manager) executeScreenshot(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	var options screenshotOptions
	if argsJSON != "" {
		var err error
		options, err = tooling.ParseArgs[screenshotOptions](argsJSON)
		if err != nil {
			return "", err
		}
	}
	yes := true
	options.Screenshot = &yes
	b, err := m.open(env)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	return finishAction(b, env, "captured screenshot", options)
}
