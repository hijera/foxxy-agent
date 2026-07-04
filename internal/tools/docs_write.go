package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	toolfs "github.com/hijera/foxxycode-agent/internal/tools/fs"
)

// DocsWriteTool creates or overwrites a markdown documentation file within the session CWD.
func DocsWriteTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "docs_write",
			Description: "Create or overwrite a markdown documentation file (.md only). " +
				"Allowed targets include README.md, AGENTS.md, DESIGN.md, and files under docs/. " +
				"Cannot modify source code or internal/prompts templates.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Relative markdown file path (must end with .md)",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Full markdown content to write",
					},
				},
				"required": []interface{}{"path", "content"},
			},
		},
		RequiresPermission: false,
		Execute:            executeDocsWrite,
	}
}

type docsWriteArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func executeDocsWrite(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[docsWriteArgs](argsJSON)
	if err != nil {
		return "", err
	}

	path, err := resolveDocsPath(args.Path, env.CWD)
	if err != nil {
		return "", fmt.Errorf("docs_write: %w", err)
	}

	var before []byte
	if env != nil && env.OnFileEdit != nil {
		before, _ = os.ReadFile(path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("docs_write mkdir: %w", err)
	}
	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("docs_write: %w", err)
	}

	toolfs.NotifyFileEdit(env, "docs_write", path, before, []byte(args.Content))

	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), path), nil
}
