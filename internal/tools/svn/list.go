package svn

import (
	"context"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// ListTool lists repository entries, typically to discover branches.
func (c client) ListTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_list",
			Description: "List Subversion repository entries (svn list). Use it to discover branches: pass \"branches\" to list the repository's branches/ directory, " +
				"or a full repository URL. Empty lists the current working copy directory.",
			InputSchema: objectSchema(map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Repository URL, a working copy path, or one of the shortcuts \"trunk\", \"branches\", \"tags\".",
				},
			}, nil),
		},
		Execute: c.executeList,
	}
}

type listArgs struct {
	Target string `json:"target"`
}

func (c client) executeList(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[listArgs](argsJSON)
	if err != nil {
		return "", err
	}
	info, err := c.requireWorkingCopy(ctx, env)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	target := strings.TrimSpace(args.Target)
	switch target {
	case "trunk", "branches", "tags":
		root := strings.TrimSuffix(info.RepositoryRoot, "/")
		if root == "" {
			return "error: repository root unknown for this working copy", nil
		}
		target = root + "/" + target
	}
	out, err := svnws.List(ctx, cwd(env), c.opts, target)
	return report(out, err, "no entries")
}
