//go:build browser

package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// findScanLimit is how deep the page walk goes when searching. Larger than the
// read_page outline cap because the caller sees only the matches, not the walk.
const findScanLimit = 2000

// findMaxMatches bounds the report.
const findMaxMatches = 40

// FindTool searches the page for elements whose role, name, value or selector
// contains a query, and reports them in outline form. On a large page this
// replaces reading a 400-line outline to locate one control.
func (m *Manager) FindTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_find",
			Description: "Find elements on the current interactive browser page whose role, visible text, value or selector contains " +
				"the query (case-insensitive). Reports matching outline lines with CSS selectors to act on. Takes no screenshot. " +
				"Use it instead of foxxycode_browser_read_page when looking for one specific control on a large page.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Text to look for, e.g. a button label, placeholder, or field name.",
					},
				},
				"required": []string{"query"},
			},
		},
		RequiresPermission: false,
		Execute:            m.executeFind,
	}
}

type findArgs struct {
	Query string `json:"query"`
}

func (m *Manager) executeFind(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[findArgs](argsJSON)
	if err != nil {
		return "", err
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "error: query is required", nil
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	script := fmt.Sprintf(readPageScript, false, findScanLimit)
	var res readPageResult
	if err := b.run(chromedp.Evaluate(script, &res)); err != nil {
		return fmt.Sprintf("error: find: %v", err), nil
	}

	needle := strings.ToLower(query)
	var matches []readPageNode
	for _, n := range res.Nodes {
		hay := strings.ToLower(n.Role + " " + n.Name + " " + n.Value + " " + n.Selector)
		if strings.Contains(hay, needle) {
			matches = append(matches, n)
			if len(matches) >= findMaxMatches {
				break
			}
		}
	}
	if len(matches) == 0 {
		return fmt.Sprintf("no elements match %q (scanned %d elements)", query, len(res.Nodes)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d element(s) match %q:\n", len(matches), query)
	for _, n := range matches {
		fmt.Fprintf(&sb, "%s", n.Role)
		if n.Name != "" {
			fmt.Fprintf(&sb, " %q", n.Name)
		}
		if n.Value != "" {
			fmt.Fprintf(&sb, " value=%q", n.Value)
		}
		if n.Disabled {
			sb.WriteString(" [disabled]")
		}
		if n.Selector != "" {
			fmt.Fprintf(&sb, "  selector=%s", n.Selector)
		}
		sb.WriteByte('\n')
	}
	if len(matches) >= findMaxMatches {
		sb.WriteString("[more matches not shown; narrow the query]\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
