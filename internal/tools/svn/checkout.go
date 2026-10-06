package svn

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// CheckoutTool checks a branch out into its own folder, the usual way of working
// on several SVN branches side by side.
func (c client) CheckoutTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_checkout",
			Description: "Check a Subversion branch out into its own folder (svn checkout), the branch-folder workflow. " +
				"Accepts a branch name such as \"branches/feature-x\" resolved against the current repository, or a full repository URL. " +
				"The session keeps working in its current folder; use it when you need a second branch on disk.",
			InputSchema: objectSchema(map[string]interface{}{
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Branch to check out: \"trunk\", \"branches/<name>\", or a full repository URL.",
				},
				"destination": map[string]interface{}{
					"type":        "string",
					"description": "Target folder, relative to the session folder or absolute. Defaults to a sibling folder named after the branch.",
				},
				"revision": map[string]interface{}{
					"type":        "string",
					"description": "Optional revision to check out (default HEAD).",
				},
			}, []string{"branch"}),
		},
		RequiresPermission: true,
		Execute:            c.executeCheckout,
	}
}

type checkoutArgs struct {
	Branch      string `json:"branch"`
	Destination string `json:"destination"`
	Revision    string `json:"revision"`
}

func (c client) executeCheckout(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[checkoutArgs](argsJSON)
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
	dest := checkoutDestination(cwd(env), args.Destination, args.Branch)
	out, err := svnws.Checkout(ctx, c.opts, url, dest, args.Revision)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	return strings.TrimSpace(out + "\nchecked out " + url + " into " + dest), nil
}

// checkoutDestination resolves the target folder: an explicit destination
// (relative to the session folder) or a sibling folder named after the branch.
func checkoutDestination(sessionDir, destination, branch string) string {
	dest := strings.TrimSpace(destination)
	if dest == "" {
		name := svnws.BranchDirName(branch)
		if name == "" {
			name = "checkout"
		}
		return filepath.Join(filepath.Dir(sessionDir), name)
	}
	if filepath.IsAbs(dest) {
		return filepath.Clean(dest)
	}
	return filepath.Join(sessionDir, dest)
}
