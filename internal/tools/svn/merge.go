package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// MergeTool merges another branch into the working copy.
func (c client) MergeTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_merge",
			Description: "Merge another branch into the Subversion working copy (svn merge). " +
				"Accepts a branch name such as \"trunk\" or \"branches/feature-x\", or a full repository URL. " +
				"The merge only changes the working copy: review with svn_status and svn_diff, resolve conflicts with svn_resolve, then svn_commit.",
			InputSchema: objectSchema(map[string]interface{}{
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Branch to merge from: \"trunk\", \"branches/<name>\", or a full repository URL.",
				},
				"revision": map[string]interface{}{
					"type":        "string",
					"description": "Optional revision or range to merge, e.g. \"120\" or \"110:120\". Empty merges all eligible revisions.",
				},
			}, []string{"source"}),
		},
		RequiresPermission: true,
		Execute:            c.executeMerge,
	}
}

type mergeArgs struct {
	Source   string `json:"source"`
	Revision string `json:"revision"`
}

func (c client) executeMerge(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[mergeArgs](argsJSON)
	if err != nil {
		return "", err
	}
	info, err := c.requireWorkingCopy(ctx, env)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	source, err := resolveBranchTarget(info, args.Source)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Merge(ctx, cwd(env), c.opts, source, args.Revision)
	return report(out, err, "nothing to merge from "+source)
}
