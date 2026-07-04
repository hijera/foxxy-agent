//go:build scheduler

package schedtools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hijera/foxxycode-agent/external/scheduler/service"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func jobRunsTool(cfg *config.Config) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: toolJobRuns,
			Description: "Lists recent persisted scheduler runs for a job_id (metadata only). Each row includes session_id; read full turns with normal session tools or HTTP /foxxycode/sessions/{session_id}/messages. " +
				"Use when the user wants history, audit, or to debug a recurring job. Optional limit (default 50, max 100).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{"type": "string"},
					"limit":  map[string]interface{}{"type": "integer", "description": "Max rows to return"},
				},
				"required": []interface{}{"job_id"},
			},
		},
		Execute: func(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
			var in struct {
				JobID string `json:"job_id"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
				return "", err
			}
			op := schedservice.NewService(cfg, nil, toolEnvCWD(env))
			runs, err := op.ListJobRuns(strings.TrimSpace(in.JobID), in.Limit)
			if err != nil {
				return "", err
			}
			wrap := map[string]interface{}{
				"object": "foxxycode.scheduler_job_runs",
				"job_id": strings.TrimSpace(in.JobID),
				"runs":   runs,
			}
			b, err := json.MarshalIndent(wrap, "", "  ")
			return string(b), err
		},
	}
}
