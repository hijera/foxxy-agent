package svn

import (
	"context"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// InfoTool reports the working copy branch, URL, and revision.
func (c client) InfoTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "svn_info",
			Description: "Describe the Subversion working copy of the session folder: branch (trunk, branches/<name>), repository URL, working copy root, and revision. " +
				"Use it before any other svn tool to confirm which branch folder you are in.",
			InputSchema: objectSchema(map[string]interface{}{}, nil),
		},
		Execute: c.executeInfo,
	}
}

func (c client) executeInfo(ctx context.Context, _ string, env *tooling.Env) (string, error) {
	info, err := c.requireWorkingCopy(ctx, env)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "branch: %s\n", orUnknown(info.Branch))
	fmt.Fprintf(&b, "url: %s\n", info.URL)
	fmt.Fprintf(&b, "repository root: %s\n", info.RepositoryRoot)
	fmt.Fprintf(&b, "working copy root: %s\n", info.WCRoot)
	fmt.Fprintf(&b, "revision: %d\n", info.Revision)
	if info.Nested {
		fmt.Fprintf(&b, "note: %s is not versioned itself; the working copy root is above it\n", info.Path)
	}
	if len(info.Branches) > 0 {
		fmt.Fprintf(&b, "branches: %s\n", strings.Join(info.Branches, ", "))
	}
	return strings.TrimSpace(b.String()), nil
}

func orUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(unrecognised layout)"
	}
	return v
}
