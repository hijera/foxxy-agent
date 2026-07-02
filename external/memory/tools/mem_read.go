//go:build memory

package memtools

import (
	"context"
	"encoding/json"

	memstorage "github.com/hijera/foxxy-agent/external/memory/storage"
	"github.com/hijera/foxxy-agent/internal/llm"
	"github.com/hijera/foxxy-agent/internal/tooling"
)

func memoryReadTool(store *memstorage.Store) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        NameRead,
			Description: "Read one memory file by scope:relative path.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "Path as scope:relative, e.g. global:preferences.md or global:notes/habits.md"},
				},
				"required": []interface{}{"path"},
			},
		},
		Execute: func(ctx context.Context, argsJSON string, _ *tooling.Env) (string, error) {
			_ = ctx
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", err
			}
			return store.Read(args.Path)
		},
	}
}
