package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// ResolveTool marks conflicted paths as resolved.
func (c client) ResolveTool() *tooling.Tool {
	accepts := make([]interface{}, 0, len(svnws.AcceptValues))
	for _, v := range svnws.AcceptValues {
		accepts = append(accepts, v)
	}
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_resolve",
			Description: "Mark conflicted paths as resolved after a merge or update (svn resolve --accept). " +
				"Use \"working\" once you have edited the conflicted files yourself.",
			InputSchema: objectSchema(map[string]interface{}{
				"paths": pathsSchema("Conflicted paths to resolve, relative to the session folder."),
				"accept": map[string]interface{}{
					"type":        "string",
					"description": "Conflict resolution to apply (default \"working\": keep the file as it currently stands).",
					"enum":        accepts,
				},
			}, []string{"paths"}),
		},
		RequiresPermission: true,
		Execute:            c.executeResolve,
	}
}

type resolveArgs struct {
	Paths  []string `json:"paths"`
	Accept string   `json:"accept"`
}

func (c client) executeResolve(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[resolveArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if _, err := c.requireWorkingCopy(ctx, env); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Resolve(ctx, cwd(env), c.opts, pathsArg(args.Paths), args.Accept)
	return report(out, err, "nothing to resolve")
}
