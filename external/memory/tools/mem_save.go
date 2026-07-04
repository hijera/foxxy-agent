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

func memorySaveTool(store *memstorage.Store) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        NameSave,
			Description: "Write or overwrite a distilled memory note. Prefer relative_path with folders for reusable organization.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string", "description": "Short title; used for default flat filename when relative_path is omitted"},
					"body":  map[string]interface{}{"type": "string", "description": "Markdown or plain text body to store"},
					"scope": map[string]interface{}{"type": "string", "enum": []interface{}{"global", "project"}},
					"relative_path": map[string]interface{}{
						"type":        "string",
						"description": "Optional path under scope root with .md or .txt extension, e.g. design/auth-flow.md. When omitted, a slug from title is written at scope root.",
					},
				},
				"required": []interface{}{"title", "body", "scope"},
			},
		},
		Execute: func(ctx context.Context, argsJSON string, _ *tooling.Env) (string, error) {
			_ = ctx
			var args struct {
				Title        string `json:"title"`
				Body         string `json:"body"`
				Scope        string `json:"scope"`
				RelativePath string `json:"relative_path"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", err
			}
			body := strings.TrimSpace(args.Body)
			if len(body) > 900 {
				body = body[:900] + "\n..."
			}
			rel := strings.TrimSpace(args.RelativePath)
			p, err := store.WriteFlexible(args.Scope, args.Title, rel, body)
			if err != nil {
				return "", err
			}
			return "saved as " + p, nil
		},
	}
}
