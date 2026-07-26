package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// CommitTool sends working copy changes to the repository.
func (c client) CommitTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_commit",
			Description: "Commit Subversion changes to the repository (svn commit). Paths are mandatory and are the only thing committed, " +
				"so unrelated parts of the branch folder - including a nested git repository - are never swept in. " +
				"Run svn_status and svn_diff first and confirm the change set.",
			InputSchema: objectSchema(map[string]interface{}{
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Commit message.",
				},
				"paths": pathsSchema("Paths to commit, relative to the session folder."),
			}, []string{"message", "paths"}),
		},
		RequiresPermission: true,
		Execute:            c.executeCommit,
	}
}

type commitArgs struct {
	Message string   `json:"message"`
	Paths   []string `json:"paths"`
}

func (c client) executeCommit(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[commitArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if _, err := c.requireWorkingCopy(ctx, env); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Commit(ctx, cwd(env), c.opts, args.Message, pathsArg(args.Paths))
	return report(out, err, "nothing to commit")
}
