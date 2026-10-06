package svn

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// logDefaultLimit caps history output when the model does not ask for a size.
const logDefaultLimit = 20

// LogTool shows the revision history.
func (c client) LogTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "svn_log",
			Description: "Show Subversion revision history (svn log -v) for the working copy or a specific path or URL.",
			InputSchema: objectSchema(map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Optional path (relative to the session folder) or repository URL. Empty logs the whole working copy.",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of revisions to return (default 20).",
				},
			}, nil),
		},
		Execute: c.executeLog,
	}
}

type logArgs struct {
	Target string `json:"target"`
	Limit  int    `json:"limit"`
}

func (c client) executeLog(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[logArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if _, err := c.requireWorkingCopy(ctx, env); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	limit := args.Limit
	if limit <= 0 {
		limit = logDefaultLimit
	}
	out, err := svnws.Log(ctx, cwd(env), c.opts, args.Target, limit)
	return report(out, err, "no revisions")
}
