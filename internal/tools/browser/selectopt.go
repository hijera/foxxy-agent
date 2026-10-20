//go:build browser

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// SelectOptionTool works native <select> elements: lists their options and picks
// one, dispatching the input/change events an app listens to. SetValue-style
// fills cannot enumerate options or match by visible label.
func (m *Manager) SelectOptionTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_select_option",
			Description: "Work a <select> dropdown in the interactive browser. With only a selector it lists the options; " +
				"with value or label it picks the matching option and fires input/change events. " +
				"Returns the resulting page state and a screenshot after picking.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the <select> element.",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "Option value to select (exact match). Takes precedence over label.",
					},
					"label": map[string]interface{}{
						"type":        "string",
						"description": "Visible option label to select (exact match, used when value is empty).",
					},
				},
				"required": []string{"selector"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeSelectOption,
	}
}

type selectOptionArgs struct {
	Selector string `json:"selector"`
	Value    string `json:"value"`
	Label    string `json:"label"`
}

// selectOptionScript lists or picks an option. Selection goes through
// selectedIndex plus synthetic input/change events, which is what frameworks
// listen to; setting .value alone notifies nobody.
const selectOptionScript = `(function(spec){
  var el = document.querySelector(spec.selector);
  if (!el) return {error: "no element matches " + spec.selector};
  if (el.tagName.toLowerCase() !== "select") return {error: spec.selector + " is not a <select>"};
  var opts = [];
  for (var i = 0; i < el.options.length; i++) {
    var o = el.options[i];
    opts.push({value: o.value, label: (o.label || o.textContent || "").trim(), selected: o.selected});
  }
  if (!spec.value && !spec.label) return {options: opts};
  var idx = -1;
  for (var j = 0; j < el.options.length; j++) {
    var c = el.options[j];
    if (spec.value !== "" && c.value === spec.value) { idx = j; break; }
    if (spec.value === "" && (c.label || c.textContent || "").trim() === spec.label) { idx = j; break; }
  }
  if (idx < 0) return {error: "no option matches", options: opts};
  el.selectedIndex = idx;
  el.dispatchEvent(new Event("input", {bubbles: true}));
  el.dispatchEvent(new Event("change", {bubbles: true}));
  var s = el.options[idx];
  return {picked: {value: s.value, label: (s.label || s.textContent || "").trim()}};
})(%s)`

type selectOptionResult struct {
	Error   string `json:"error"`
	Options []struct {
		Value    string `json:"value"`
		Label    string `json:"label"`
		Selected bool   `json:"selected"`
	} `json:"options"`
	Picked *struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"picked"`
}

func (m *Manager) executeSelectOption(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[selectOptionArgs](argsJSON)
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

	spec, _ := json.Marshal(map[string]string{"selector": sel, "value": args.Value, "label": args.Label})
	var res selectOptionResult
	if err := b.run(chromedp.Evaluate(fmt.Sprintf(selectOptionScript, string(spec)), &res)); err != nil {
		return fmt.Sprintf("error: select option: %v", err), nil
	}
	if res.Error != "" && res.Picked == nil && len(res.Options) == 0 {
		return "error: " + res.Error, nil
	}

	if res.Picked != nil {
		return finishAction(b, env, fmt.Sprintf("selected option value=%q label=%q in %s", res.Picked.Value, res.Picked.Label, sel))
	}

	var sb strings.Builder
	if res.Error != "" {
		fmt.Fprintf(&sb, "error: %s; ", res.Error)
	}
	fmt.Fprintf(&sb, "options of %s (%d):\n", sel, len(res.Options))
	for _, o := range res.Options {
		mark := " "
		if o.Selected {
			mark = "*"
		}
		fmt.Fprintf(&sb, "%s value=%q label=%q\n", mark, o.Value, o.Label)
	}
	sb.WriteString("(* = currently selected; call again with value or label to pick)")
	return sb.String(), nil
}
