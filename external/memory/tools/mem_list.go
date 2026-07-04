//go:build memory

package memtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	memstorage "github.com/hijera/foxxycode-agent/external/memory/storage"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func memoryListTool(store *memstorage.Store) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        NameList,
			Description: "List directories and memory files (.md/.txt) one level under a scope-relative path. Use foxxycode_memory_mkdir before saving into a new folder.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "Directory as scope:relative, e.g. global: or global:design (list one level)"},
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
			nodes, err := store.ListOneLevel(scopeKey, inner)
			if err != nil {
				return "", err
			}
			if len(nodes) == 0 {
				return "(empty directory)", nil
			}
			var b strings.Builder
			for _, n := range nodes {
				line := fmt.Sprintf("- %s (%s)", n.Name, n.Kind)
				if n.Kind == "file" && n.Size > 0 {
					line += fmt.Sprintf(" size=%d", n.Size)
				}
				if n.Modified != "" {
					line += " modified=" + n.Modified
				}
				b.WriteString(line)
				b.WriteByte('\n')
			}
			return b.String(), nil
		},
	}
}
