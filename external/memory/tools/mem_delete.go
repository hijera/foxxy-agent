//go:build memory

package memtools

import (
	"context"
	"encoding/json"

	memstorage "github.com/hijera/foxxycode-agent/external/memory/storage"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func memoryDeleteTool(store *memstorage.Store) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        NameDelete,
			Description: "Delete a memory file or directory tree (recursive) under the scope root when the user asked to forget it or it is obsolete. Cannot delete the scope root directory.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "scope:relative path to delete"},
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
			if err := store.Delete(args.Path); err != nil {
				return "", err
			}
			return "deleted " + args.Path, nil
		},
	}
}
