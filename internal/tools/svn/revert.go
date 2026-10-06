package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// RevertTool discards local changes to the given paths.
func (c client) RevertTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "svn_revert",
			Description: "Discard local Subversion changes for the given paths (svn revert). This destroys uncommitted work in those paths.",
			InputSchema: objectSchema(map[string]interface{}{
				"paths":     pathsSchema("Paths to revert, relative to the session folder."),
				"recursive": map[string]interface{}{"type": "boolean", "description": "Revert directories recursively."},
			}, []string{"paths"}),
		},
		RequiresPermission: true,
		Execute:            c.executeRevert,
	}
}

type revertArgs struct {
	Paths     []string `json:"paths"`
	Recursive bool     `json:"recursive"`
}

func (c client) executeRevert(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[revertArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if _, err := c.requireWorkingCopy(ctx, env); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Revert(ctx, cwd(env), c.opts, pathsArg(args.Paths), args.Recursive)
	return report(out, err, "nothing reverted")
}
