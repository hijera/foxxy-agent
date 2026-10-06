package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// UpdateTool brings the working copy up to date with the repository.
func (c client) UpdateTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_update",
			Description: "Bring the Subversion working copy up to date with the repository (svn update). " +
				"Run it before committing or merging so the branch folder is current.",
			InputSchema: objectSchema(map[string]interface{}{
				"paths": pathsSchema("Optional paths to update, relative to the session folder. Empty updates the whole working copy."),
				"revision": map[string]interface{}{
					"type":        "string",
					"description": "Optional target revision (default HEAD).",
				},
			}, nil),
		},
		RequiresPermission: true,
		Execute:            c.executeUpdate,
	}
}

type updateArgs struct {
	Paths    []string `json:"paths"`
	Revision string   `json:"revision"`
}

func (c client) executeUpdate(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[updateArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if _, err := c.requireWorkingCopy(ctx, env); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Update(ctx, cwd(env), c.opts, pathsArg(args.Paths), args.Revision)
	return report(out, err, "already up to date")
}
