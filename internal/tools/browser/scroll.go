//go:build browser

package browser

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// ScrollTool scrolls the page, either to an element (selector) or by a pixel offset.
func (m *Manager) ScrollTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "foxxycode_browser_scroll",
			Description: "Scroll the interactive browser page. Provide a CSS selector to scroll that element into view, or x/y pixel offsets to scroll the window. Returns the resulting page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "Optional CSS selector to scroll into view. Takes precedence over x/y.",
					},
					"x": map[string]interface{}{
						"type":        "integer",
						"description": "Optional horizontal scroll offset in pixels (used when selector is empty).",
					},
					"y": map[string]interface{}{
						"type":        "integer",
						"description": "Optional vertical scroll offset in pixels (used when selector is empty).",
					},
				},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeScroll,
	}
}

type scrollArgs struct {
	Selector string `json:"selector"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
}

func (m *Manager) executeScroll(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[scrollArgs](argsJSON)
	if err != nil {
		return "", err
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	sel := strings.TrimSpace(args.Selector)
	var js, desc string
	if sel != "" {
		js = fmt.Sprintf(`(function(){var el=document.querySelector(%s);`+
			`if(!el){return false;}el.scrollIntoView({block:'center',inline:'center'});return true;})()`, strconv.Quote(sel))
		desc = "scrolled to " + sel
	} else {
		js = fmt.Sprintf(`(function(){window.scrollTo(%d,%d);return true;})()`, args.X, args.Y)
		desc = fmt.Sprintf("scrolled to (%d,%d)", args.X, args.Y)
	}

	var ok bool
	if err := b.run(chromedp.Evaluate(js, &ok)); err != nil {
		return fmt.Sprintf("error: scroll: %v", err), nil
	}
	if sel != "" && !ok {
		return fmt.Sprintf("error: scroll %q: element not found", sel), nil
	}
	return finishAction(b, env, desc)
}
