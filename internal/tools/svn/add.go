package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// AddTool schedules new files for addition.
func (c client) AddTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_add",
			Description: "Schedule files or directories for addition to Subversion (svn add --parents). " +
				"Only the listed paths are added, so unversioned neighbours such as a nested git clone stay untouched.",
			InputSchema: objectSchema(map[string]interface{}{
				"paths": pathsSchema("Paths to add, relative to the session folder."),
			}, []string{"paths"}),
		},
		RequiresPermission: true,
		Execute:            c.executeAdd,
	}
}

func (c client) executeAdd(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[pathsOnlyArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if _, err := c.requireWorkingCopy(ctx, env); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Add(ctx, cwd(env), c.opts, pathsArg(args.Paths))
	return report(out, err, "nothing added")
}
