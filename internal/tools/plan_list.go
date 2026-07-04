package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// PlanListTool lists design plan files in the session bundle.
func PlanListTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "plan_list",
			Description: "List design plan files (plans/*.plan.md) in the current session bundle.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		RequiresPermission: false,
		Execute:            executePlanList,
	}
}

func executePlanList(_ context.Context, _ string, env *tooling.Env) (string, error) {
	sd := strings.TrimSpace(env.SessionDir)
	if sd == "" {
		return "", fmt.Errorf("plan_list: session persistence is not available")
	}
	items, err := plans.List(sd)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "No design plans in this session.", nil
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
