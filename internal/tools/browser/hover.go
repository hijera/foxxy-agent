//go:build browser

package browser

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// HoverTool moves the mouse over the element matching a CSS selector, triggering
// real :hover styles and mouseover handlers.
func (m *Manager) HoverTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "foxxycode_browser_hover",
			Description: "Move the mouse over the element matching a CSS selector in the interactive browser (reveals hover menus/tooltips). Returns the resulting page state and a screenshot.",
			InputSchema: selectorSchema("CSS selector of the element to hover over."),
		},
		RequiresPermission: true,
		Execute:            m.executeHover,
	}
}

func (m *Manager) executeHover(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[selectorArgs](argsJSON)
	if err != nil {
		return "", err
	}
	sel := strings.TrimSpace(args.Selector)
	if sel == "" {
		return "error: selector is required", nil
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	var pt struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
	}
	js := fmt.Sprintf(`(function(){var el=document.querySelector(%s);`+
		`if(!el){return{found:false,x:0,y:0};}`+
		`el.scrollIntoView({block:'center',inline:'center'});`+
		`var r=el.getBoundingClientRect();`+
		`return{found:true,x:r.left+r.width/2,y:r.top+r.height/2};})()`, strconv.Quote(sel))
	if err := b.run(chromedp.Evaluate(js, &pt)); err != nil {
		return fmt.Sprintf("error: hover %q: %v", sel, err), nil
	}
	if !pt.Found {
		return fmt.Sprintf("error: hover %q: element not found", sel), nil
	}
	if err := b.run(chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, pt.X, pt.Y).Do(ctx)
	})); err != nil {
		return fmt.Sprintf("error: hover %q: %v", sel, err), nil
	}
	return finishAction(b, env, "hovered "+sel)
}
