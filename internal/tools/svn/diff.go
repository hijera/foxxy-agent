package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// DiffTool shows working copy changes as a unified diff.
func (c client) DiffTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_diff",
			Description: "Show a unified diff of Subversion changes (svn diff). Without a revision it diffs the working copy against its base; " +
				"with a revision or range (for example \"12\" or \"12:HEAD\") it diffs those revisions.",
			InputSchema: objectSchema(map[string]interface{}{
				"paths": pathsSchema("Optional paths to diff, relative to the session folder. Empty diffs the whole working copy."),
				"revision": map[string]interface{}{
					"type":        "string",
					"description": "Optional revision or range, e.g. \"12\", \"12:HEAD\", \"BASE:HEAD\".",
				},
			}, nil),
		},
		Execute: c.executeDiff,
	}
}

type diffArgs struct {
	Paths    []string `json:"paths"`
	Revision string   `json:"revision"`
}

func (c client) executeDiff(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[diffArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if _, err := c.requireWorkingCopy(ctx, env); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Diff(ctx, cwd(env), c.opts, pathsArg(args.Paths), args.Revision)
	return report(out, err, "no differences")
}
