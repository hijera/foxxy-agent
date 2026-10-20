//go:build browser

package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// resize bounds: below ~200px Chrome renders nothing useful, above 4000 the
// screenshots explode in size for no diagnostic gain.
const (
	resizeMin = 200
	resizeMax = 4000
)

// ResizeTool emulates a viewport size, which is how responsive layouts (narrow
// 390px vs wide 1280px) are verified without a window manager.
func (m *Manager) ResizeTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_resize",
			Description: "Set the interactive browser's viewport size (CSS pixels), e.g. 390x844 for a phone or 1280x800 for desktop. " +
				"Pass width=0 and height=0 to reset to the default. Returns the resulting page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"width": map[string]interface{}{
						"type":        "integer",
						"description": fmt.Sprintf("Viewport width in pixels (%d-%d, or 0 with height 0 to reset).", resizeMin, resizeMax),
					},
					"height": map[string]interface{}{
						"type":        "integer",
						"description": fmt.Sprintf("Viewport height in pixels (%d-%d, or 0 with width 0 to reset).", resizeMin, resizeMax),
					},
				},
				"required": []interface{}{"width", "height"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeResize,
	}
}

type resizeArgs struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (m *Manager) executeResize(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[resizeArgs](argsJSON)
	if err != nil {
		return "", err
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	if args.Width == 0 && args.Height == 0 {
		if err := b.run(chromedp.ResetViewport()); err != nil {
			return fmt.Sprintf("error: reset viewport: %v", err), nil
		}
		return finishAction(b, env, "viewport reset to default")
	}
	if args.Width < resizeMin || args.Width > resizeMax || args.Height < resizeMin || args.Height > resizeMax {
		return fmt.Sprintf("error: width and height must be within %d-%d (got %dx%d), or both 0 to reset",
			resizeMin, resizeMax, args.Width, args.Height), nil
	}
	if err := b.run(chromedp.EmulateViewport(int64(args.Width), int64(args.Height))); err != nil {
		return fmt.Sprintf("error: resize viewport: %v", err), nil
	}
	return finishAction(b, env, fmt.Sprintf("viewport set to %dx%d", args.Width, args.Height))
}
