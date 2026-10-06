package svn

import (
	"context"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// SwitchTool repoints the working copy at another branch, in place.
func (c client) SwitchTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_switch",
			Description: "Point the Subversion working copy at another branch in place (svn switch). " +
				"Accepts a branch name such as \"trunk\" or \"branches/feature-x\", or a full repository URL. " +
				"To work on a branch in its own folder instead, use svn_checkout.",
			InputSchema: objectSchema(map[string]interface{}{
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Branch to switch to: \"trunk\", \"branches/<name>\", \"tags/<name>\", or a full repository URL.",
				},
			}, []string{"branch"}),
		},
		RequiresPermission: true,
		Execute:            c.executeSwitch,
	}
}

type switchArgs struct {
	Branch string `json:"branch"`
}

func (c client) executeSwitch(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[switchArgs](argsJSON)
	if err != nil {
		return "", err
	}
	info, err := c.requireWorkingCopy(ctx, env)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	url, err := resolveBranchTarget(info, args.Branch)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	out, err := svnws.Switch(ctx, cwd(env), c.opts, url)
	return report(out, err, "switched to "+url)
}

// resolveBranchTarget accepts either a full repository URL or a branch name
// relative to the repository root.
func resolveBranchTarget(info svnws.Info, branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("branch is required")
	}
	if strings.Contains(branch, "://") {
		return branch, nil
	}
	return svnws.BranchURL(info, branch)
}
