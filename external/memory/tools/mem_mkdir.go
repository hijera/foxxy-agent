//go:build memory

package memtools

import (
	"context"
	"encoding/json"
	"strings"

	memstorage "github.com/hijera/foxxycode-agent/external/memory/storage"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func memoryMkdirTool(store *memstorage.Store) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        NameMkdir,
			Description: "Create nested directories under a memory scope (idempotent). Use thematic folders before first save.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "Directory to create under scope, e.g. global:preferences or project:architecture/api"},
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
			scopeKey, inner, err := ParseScopeColonPath(args.Path)
			if err != nil {
				return "", err
			}
			if err := store.Mkdir(scopeKey, inner); err != nil {
				return "", err
			}
			return "created or exists: " + strings.TrimSpace(args.Path), nil
		},
	}
}
