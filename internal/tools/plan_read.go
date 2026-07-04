package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// PlanReadTool reads a design plan file from the session bundle by slug.
func PlanReadTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "plan_read",
			Description: "Read a design plan file (plans/<slug>.plan.md) from the current session bundle. " +
				"Returns the full file including YAML frontmatter and markdown body. " +
				"Use plan_list to discover slugs; do not use read on session plan paths.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"slug": map[string]interface{}{
						"type":        "string",
						"description": "Plan identifier (same slug used with plan_write)",
					},
				},
				"required": []interface{}{"slug"},
			},
		},
		RequiresPermission: false,
		Execute:            executePlanRead,
	}
}

type planReadArgs struct {
	Slug string `json:"slug"`
}

func executePlanRead(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[planReadArgs](argsJSON)
	if err != nil {
		return "", err
	}
	slug := strings.TrimSpace(args.Slug)
	if err := plans.ValidateSlug(slug); err != nil {
		return "", err
	}
	sd := strings.TrimSpace(env.SessionDir)
	if sd == "" {
		return "", fmt.Errorf("plan_read: session persistence is not available")
	}
	doc, err := plans.Read(sd, slug)
	if err != nil {
		if errors.Is(err, plans.ErrNotFound) {
			return "", fmt.Errorf("plan_read: plan %q not found (use plan_list)", slug)
		}
		return "", err
	}
	return doc.Content, nil
}
