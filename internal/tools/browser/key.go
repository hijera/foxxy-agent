//go:build browser

package browser

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// PressKeyTool sends real keyboard events to the page. It exists because fill
// replaces a field's value without any key events: submit-on-Enter, Escape-to-
// close and arrow-key navigation are unreachable without it.
func (m *Manager) PressKeyTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_press_key",
			Description: "Press a keyboard key in the interactive browser (real key events, unlike foxxycode_browser_fill). " +
				"Named keys: Enter, Escape, Tab, Backspace, Delete, Space, ArrowUp/Down/Left/Right, Home, End, PageUp, PageDown; " +
				"a single character sends that character; combos like \"Control+a\" apply modifiers (Control, Alt, Shift, Meta). " +
				"Give a CSS selector to focus an element first. Returns the resulting page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Key to press, e.g. \"Enter\", \"Escape\", \"a\", \"Control+a\".",
					},
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "Optional CSS selector of the element to focus before pressing.",
					},
				},
				"required": []string{"key"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executePressKey,
	}
}

type pressKeyArgs struct {
	Key      string `json:"key"`
	Selector string `json:"selector"`
}

// namedKeys maps human key names (lower-cased) to the kb package's encodings.
var namedKeys = map[string]string{
	"enter":      kb.Enter,
	"return":     kb.Enter,
	"escape":     kb.Escape,
	"esc":        kb.Escape,
	"tab":        kb.Tab,
	"backspace":  kb.Backspace,
	"delete":     kb.Delete,
	"del":        kb.Delete,
	"space":      " ",
	"arrowup":    kb.ArrowUp,
	"up":         kb.ArrowUp,
	"arrowdown":  kb.ArrowDown,
	"down":       kb.ArrowDown,
	"arrowleft":  kb.ArrowLeft,
	"left":       kb.ArrowLeft,
	"arrowright": kb.ArrowRight,
	"right":      kb.ArrowRight,
	"home":       kb.Home,
	"end":        kb.End,
	"pageup":     kb.PageUp,
	"pagedown":   kb.PageDown,
}

var keyModifiers = map[string]input.Modifier{
	"control": input.ModifierCtrl,
	"ctrl":    input.ModifierCtrl,
	"alt":     input.ModifierAlt,
	"shift":   input.ModifierShift,
	"meta":    input.ModifierMeta,
	"cmd":     input.ModifierMeta,
}

// parseKeySpec turns "Control+a" / "Enter" / "x" into the kb-encoded key string
// plus the modifier set.
func parseKeySpec(spec string) (string, []input.Modifier, error) {
	parts := strings.Split(spec, "+")
	// A bare "+" is the plus character, not an empty combo.
	if spec == "+" {
		return "+", nil, nil
	}
	var mods []input.Modifier
	for _, p := range parts[:len(parts)-1] {
		mod, ok := keyModifiers[strings.ToLower(strings.TrimSpace(p))]
		if !ok {
			return "", nil, fmt.Errorf("unknown modifier %q (use Control, Alt, Shift, Meta)", p)
		}
		mods = append(mods, mod)
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if named, ok := namedKeys[strings.ToLower(last)]; ok {
		return named, mods, nil
	}
	if utf8.RuneCountInString(last) == 1 {
		return last, mods, nil
	}
	return "", nil, fmt.Errorf("unknown key %q (use a named key like Enter/Escape/Tab or a single character)", last)
}

func (m *Manager) executePressKey(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[pressKeyArgs](argsJSON)
	if err != nil {
		return "", err
	}
	spec := strings.TrimSpace(args.Key)
	if spec == "" {
		return "error: key is required", nil
	}
	keys, mods, err := parseKeySpec(spec)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	actions := make([]chromedp.Action, 0, 3)
	sel := strings.TrimSpace(args.Selector)
	if sel != "" {
		actions = append(actions,
			chromedp.WaitVisible(sel, chromedp.ByQuery),
			chromedp.Focus(sel, chromedp.ByQuery),
		)
	}
	var opts []chromedp.KeyOption
	if len(mods) > 0 {
		opts = append(opts, chromedp.KeyModifiers(mods...))
	}
	actions = append(actions, chromedp.KeyEvent(keys, opts...))
	if err := b.run(actions...); err != nil {
		return fmt.Sprintf("error: press %q: %v", spec, err), nil
	}
	desc := "pressed " + spec
	if sel != "" {
		desc += " in " + sel
	}
	return finishAction(b, env, desc)
}
