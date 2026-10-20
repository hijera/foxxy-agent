//go:build browser

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// DragTool performs a pointer drag from one element (or point) to another. It
// dispatches real mouse events, which is what sortable lists and drag handles
// built on pointer/mouse events respond to. Native HTML5 drag-and-drop (the
// dragstart/drop event family) does not run on synthetic mouse events; the
// description says so rather than letting the model retry forever.
func (m *Manager) DragTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_drag",
			Description: "Drag with the mouse in the interactive browser: press on the source, move, release on the target. " +
				"Give CSS selectors (from_selector/to_selector) or viewport coordinates (from_x/from_y/to_x/to_y). " +
				"Works pointer-event based UIs (sortable lists, sliders, canvases); native HTML5 dragstart/drop APIs may not react. " +
				"Returns the resulting page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"from_selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the element to drag (its center is used).",
					},
					"to_selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the drop target (its center is used).",
					},
					"from_x": map[string]interface{}{"type": "number", "description": "Start x in viewport pixels (when from_selector is empty)."},
					"from_y": map[string]interface{}{"type": "number", "description": "Start y in viewport pixels."},
					"to_x":   map[string]interface{}{"type": "number", "description": "End x in viewport pixels (when to_selector is empty)."},
					"to_y":   map[string]interface{}{"type": "number", "description": "End y in viewport pixels."},
				},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeDrag,
	}
}

type dragArgs struct {
	FromSelector string   `json:"from_selector"`
	ToSelector   string   `json:"to_selector"`
	FromX        *float64 `json:"from_x"`
	FromY        *float64 `json:"from_y"`
	ToX          *float64 `json:"to_x"`
	ToY          *float64 `json:"to_y"`
}

// elementCenterScript scrolls an element into view and reports its center.
const elementCenterScript = `(function(sel){
  var el = document.querySelector(sel);
  if (!el) return null;
  el.scrollIntoView({block: "center", inline: "center"});
  var r = el.getBoundingClientRect();
  return {x: r.left + r.width / 2, y: r.top + r.height / 2};
})(%s)`

type pointResult struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// resolvePoint turns a selector or explicit coordinates into a viewport point.
func (b *Browser) resolvePoint(selector string, x, y *float64, role string) (pointResult, error) {
	selector = strings.TrimSpace(selector)
	if selector != "" {
		sel, _ := json.Marshal(selector)
		var pt *pointResult
		if err := b.run(chromedp.Evaluate(fmt.Sprintf(elementCenterScript, string(sel)), &pt)); err != nil {
			return pointResult{}, fmt.Errorf("%s %q: %w", role, selector, err)
		}
		if pt == nil {
			return pointResult{}, fmt.Errorf("%s: no element matches %q", role, selector)
		}
		return *pt, nil
	}
	if x == nil || y == nil {
		return pointResult{}, fmt.Errorf("%s: give a selector or both coordinates", role)
	}
	return pointResult{X: *x, Y: *y}, nil
}

func (m *Manager) executeDrag(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[dragArgs](argsJSON)
	if err != nil {
		return "", err
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	from, err := b.resolvePoint(args.FromSelector, args.FromX, args.FromY, "drag source")
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	to, err := b.resolvePoint(args.ToSelector, args.ToX, args.ToY, "drag target")
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	// Press, glide through a midpoint (drag logic often ignores a single jump),
	// then release on the target.
	mid := pointResult{X: (from.X + to.X) / 2, Y: (from.Y + to.Y) / 2}
	err = b.run(
		chromedp.MouseEvent(input.MouseMoved, from.X, from.Y),
		chromedp.MouseEvent(input.MousePressed, from.X, from.Y, chromedp.ButtonType(input.Left), chromedp.ClickCount(1)),
		chromedp.MouseEvent(input.MouseMoved, mid.X, mid.Y, chromedp.ButtonType(input.Left)),
		chromedp.MouseEvent(input.MouseMoved, to.X, to.Y, chromedp.ButtonType(input.Left)),
		chromedp.MouseEvent(input.MouseReleased, to.X, to.Y, chromedp.ButtonType(input.Left), chromedp.ClickCount(1)),
	)
	if err != nil {
		return fmt.Sprintf("error: drag: %v", err), nil
	}
	return finishAction(b, env, fmt.Sprintf("dragged from (%.0f,%.0f) to (%.0f,%.0f)", from.X, from.Y, to.X, to.Y))
}
