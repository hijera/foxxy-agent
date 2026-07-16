package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// EditTool performs exact string replacement in an existing file.
func EditTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "edit",
			Description: "Edit a file by replacing an exact contiguous range of text (oldString) with newString. If oldString is empty, new content replaces the entire file when creating from empty.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to edit",
					},
					"oldString": map[string]interface{}{
						"type":        "string",
						"description": "Text to search for (exact match). Use empty with newString to replace full file when file is empty or creating.",
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
				"required": []string{"path", "newString"},
			},
		},
		RequiresPermission: false,
		Execute:            executeEdit,
	}
}

type editArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"oldString"`
	NewString  string `json:"newString"`
	ReplaceAll *bool  `json:"replaceAll"`
}

func executeEdit(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[editArgs](argsJSON)
	if err != nil {
		return "", err
	}

	path := ResolvePath(args.Path, env.CWD)

	if args.OldString == args.NewString && args.OldString != "" {
		return "", fmt.Errorf("edit: oldString and newString must differ")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("edit: read: %w", err)
	}

	content, encoding, err := decodeText(data)
	if err != nil {
		return "", fmt.Errorf("edit: decode: %w", err)
	}

	out, err := applyEditToContent(content, args)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("edit mkdir: %w", err)
	}
	encoded, err := encodeText(out, encoding)
	if err != nil {
		return "", fmt.Errorf("edit: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return "", fmt.Errorf("edit: write: %w", err)
	}

	notifyFileEdit(env, "edit", path, data, encoded)

	return fmt.Sprintf("edited %s (%d bytes written)", path, len(encoded)), nil
}

// applyEditToContent computes the result of an edit against content without touching disk.
// Shared by executeEdit and the preview path so the preview matches the eventual write.
func applyEditToContent(content string, args editArgs) (string, error) {
	old := args.OldString
	replaceAll := args.ReplaceAll != nil && *args.ReplaceAll

	if old == "" {
		return args.NewString, nil
	}
	if replaceAll {
		if !strings.Contains(content, old) {
			return "", fmt.Errorf("edit: oldString not found in file")
		}
		return strings.ReplaceAll(content, old, args.NewString), nil
	}
	idx := strings.Index(content, old)
	if idx < 0 {
		return "", fmt.Errorf("edit: oldString not found in file")
	}
	return content[:idx] + args.NewString + content[idx+len(old):], nil
}
