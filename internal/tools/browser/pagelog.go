//go:build browser

package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// PageLogTool reports what the page said about itself — console calls, uncaught
// exceptions, and failed or error-status network responses — without touching the
// page and without capturing a screenshot.
//
// The other actions already append this to their result, but only they do, so a
// model that reads the DOM with evaluate could never see an error the page logged.
// It also needs no permission: reading what already happened changes nothing.
func (m *Manager) PageLogTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_page_log",
			Description: "Report console output, uncaught exceptions, and failed or 4xx/5xx network responses collected from the current page since they were last reported. " +
				"Takes no screenshot and does not touch the page. Use it to diagnose a page that looks blank or wrong, and after " +
				"foxxycode_browser_evaluate, which does not carry the log itself.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		RequiresPermission: false,
		Execute:            m.executePageLog,
	}
}

func (m *Manager) executePageLog(_ context.Context, _ string, env *tooling.Env) (string, error) {
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	logs := b.drainPageLog()
	if len(logs) == 0 {
		return "page log: empty (nothing reported since the last read)", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "page log (%d entries, cleared by this read):\n", len(logs))
	for _, l := range logs {
		fmt.Fprintf(&sb, "  %s\n", l)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
