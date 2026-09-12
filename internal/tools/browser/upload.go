//go:build browser

package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// UploadFileTool attaches local files to an <input type=file>. A file dialog
// cannot be driven in a headless browser, so this sets the input directly over
// CDP.
func (m *Manager) UploadFileTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_upload_file",
			Description: "Attach one or more local files to a file input (<input type=\"file\">) in the interactive browser. " +
				"Relative paths resolve against the session working directory. Returns the resulting page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the file input.",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path of the file to attach (or use paths for several).",
					},
					"paths": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Paths of the files to attach (for multi-file inputs).",
					},
				},
				"required": []string{"selector"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeUploadFile,
	}
}

type uploadFileArgs struct {
	Selector string   `json:"selector"`
	Path     string   `json:"path"`
	Paths    []string `json:"paths"`
}

func (m *Manager) executeUploadFile(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[uploadFileArgs](argsJSON)
	if err != nil {
		return "", err
	}
	sel := strings.TrimSpace(args.Selector)
	if sel == "" {
		return "error: selector is required", nil
	}
	raw := args.Paths
	if p := strings.TrimSpace(args.Path); p != "" {
		raw = append([]string{p}, raw...)
	}
	if len(raw) == 0 {
		return "error: path (or paths) is required", nil
	}

	cwd := ""
	if env != nil {
		cwd = env.CWD
	}
	files := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) && cwd != "" {
			p = filepath.Join(cwd, p)
		}
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Sprintf("error: file %q: %v", p, err), nil
		}
		if info.IsDir() {
			return fmt.Sprintf("error: %q is a directory, not a file", p), nil
		}
		files = append(files, p)
	}
	if len(files) == 0 {
		return "error: path (or paths) is required", nil
	}

	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	if err := b.run(chromedp.SetUploadFiles(sel, files, chromedp.ByQuery)); err != nil {
		return fmt.Sprintf("error: upload to %q: %v", sel, err), nil
	}
	return finishAction(b, env, fmt.Sprintf("attached %d file(s) to %s", len(files), sel))
}
