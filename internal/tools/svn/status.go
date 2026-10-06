package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// StatusTool lists local modifications in the working copy.
func (c client) StatusTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_status",
			Description: "Show Subversion working copy status (svn status). Output columns follow the svn convention: M modified, A added, D deleted, ? unversioned, C conflicted. " +
				"Unversioned entries such as a nested git clone are reported as-is.",
			InputSchema: objectSchema(map[string]interface{}{
				"paths": pathsSchema("Optional paths to inspect, relative to the session folder. Empty checks the whole working copy."),
			}, nil),
		},
		Execute: c.executeStatus,
	}
}

type pathsOnlyArgs struct {
	Paths []string `json:"paths"`
}

func (c client) executeStatus(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[pathsOnlyArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if _, err := c.requireWorkingCopy(ctx, env); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Status(ctx, cwd(env), c.opts, pathsArg(args.Paths))
	return report(out, err, "working copy is clean")
}
