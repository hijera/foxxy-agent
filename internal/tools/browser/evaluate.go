//go:build browser

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// EvaluateTool runs a JavaScript expression in the page and returns its JSON result.
func (m *Manager) EvaluateTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "foxxycode_browser_evaluate",
			Description: "Evaluate a JavaScript expression in the current page of the interactive browser and return its JSON-serialised result. Use to read DOM state (e.g. document.title, element text) or trigger page logic.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"expression": map[string]interface{}{
						"type":        "string",
						"description": "JavaScript expression to evaluate (its value is returned as JSON).",
					},
				},
				"required": []string{"expression"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeEvaluate,
	}
}

type evaluateArgs struct {
	Expression string `json:"expression"`
}

func (m *Manager) executeEvaluate(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[evaluateArgs](argsJSON)
	if err != nil {
		return "", err
	}
	expr := strings.TrimSpace(args.Expression)
	if expr == "" {
		return "error: expression is required", nil
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	var raw json.RawMessage
	if err := b.run(chromedp.Evaluate(expr, &raw)); err != nil {
		return fmt.Sprintf("error: evaluate: %v", err), nil
	}
	result := strings.TrimSpace(string(raw))
	if result == "" {
		result = "null"
	}
	return "result: " + result, nil
}
