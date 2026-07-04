package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	toolfs "github.com/hijera/foxxycode-agent/internal/tools/fs"
)

// DocsEditTool performs exact string replacement in an allowed markdown documentation file.
func DocsEditTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "docs_edit",
			Description: "Edit a markdown documentation file (.md only) by replacing an exact text range. " +
				"Allowed targets include README.md, AGENTS.md, DESIGN.md, and files under docs/. " +
				"Cannot modify source code or internal/prompts templates.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Relative markdown file path (must end with .md)",
					},
					"oldString": map[string]interface{}{
						"type":        "string",
						"description": "Text to search for (exact match)",
					},
					"newString": map[string]interface{}{
						"type":        "string",
						"description": "Replacement text",
					},
					"replaceAll": map[string]interface{}{
						"type":        "boolean",
						"description": "Replace every occurrence of oldString (default: false)",
					},
				},
				"required": []interface{}{"path", "newString"},
			},
		},
		RequiresPermission: false,
		Execute:            executeDocsEdit,
	}
}

type docsEditArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"oldString"`
	NewString  string `json:"newString"`
	ReplaceAll *bool  `json:"replaceAll"`
}

func executeDocsEdit(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[docsEditArgs](argsJSON)
	if err != nil {
		return "", err
	}

	path, err := resolveDocsPath(args.Path, env.CWD)
	if err != nil {
		return "", fmt.Errorf("docs_edit: %w", err)
	}

	if args.OldString == args.NewString && args.OldString != "" {
		return "", fmt.Errorf("docs_edit: oldString and newString must differ")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("docs_edit: read: %w", err)
	}

	out, err := applyDocsEdit(string(data), args)
	if err != nil {
		return "", fmt.Errorf("docs_edit: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("docs_edit mkdir: %w", err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return "", fmt.Errorf("docs_edit: write: %w", err)
	}

	toolfs.NotifyFileEdit(env, "docs_edit", path, data, []byte(out))

	return fmt.Sprintf("edited %s (%d bytes written)", path, len(out)), nil
}

func applyDocsEdit(content string, args docsEditArgs) (string, error) {
	old := args.OldString
	replaceAll := args.ReplaceAll != nil && *args.ReplaceAll

	if old == "" {
		return args.NewString, nil
	}
	if replaceAll {
		if !strings.Contains(content, old) {
			return "", fmt.Errorf("oldString not found in file")
		}
		return strings.ReplaceAll(content, old, args.NewString), nil
	}
	idx := strings.Index(content, old)
	if idx < 0 {
		return "", fmt.Errorf("oldString not found in file")
	}
	return content[:idx] + args.NewString + content[idx+len(old):], nil
}
