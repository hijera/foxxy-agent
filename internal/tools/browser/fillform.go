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

// FillFormTool sets several fields in one call. A form of N fields costs one
// round-trip and one screenshot instead of N of each.
func (m *Manager) FillFormTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_fill_form",
			Description: "Fill several form fields of the interactive browser page in one call. Each field takes a CSS selector and a value; " +
				"text inputs and textareas get the value with input/change events, <select> elements pick the option whose value or label matches. " +
				"Checkboxes/radios are toggled with values \"true\"/\"false\". Returns the resulting page state and one screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"fields": map[string]interface{}{
						"type":        "array",
						"description": "Fields to fill, in order.",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"selector": map[string]interface{}{"type": "string", "description": "CSS selector of the field."},
								"value":    map[string]interface{}{"type": "string", "description": "Value to set (option value or label for <select>, true/false for checkboxes)."},
							},
							"required": []string{"selector", "value"},
						},
					},
				},
				"required": []string{"fields"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeFillForm,
	}
}

type fillFormField struct {
	Selector string `json:"selector"`
	Value    string `json:"value"`
}

type fillFormArgs struct {
	Fields []fillFormField `json:"fields"`
}

// fillFieldScript sets one field the way a user's input would: focused, valued,
// and announced through input/change events. Returns "" on success or a problem
// description.
const fillFieldScript = `(function(spec){
  var el = document.querySelector(spec.selector);
  if (!el) return "no element matches " + spec.selector;
  var tag = el.tagName.toLowerCase();
  if (tag === "select") {
    var idx = -1;
    for (var i = 0; i < el.options.length; i++) {
      var o = el.options[i];
      if (o.value === spec.value || (o.label || o.textContent || "").trim() === spec.value) { idx = i; break; }
    }
    if (idx < 0) return "no option of " + spec.selector + " matches " + spec.value;
    el.selectedIndex = idx;
  } else if (el.type === "checkbox" || el.type === "radio") {
    el.checked = spec.value === "true" || spec.value === "1" || spec.value === "on";
  } else if ("value" in el) {
    el.focus();
    el.value = spec.value;
  } else if (el.isContentEditable) {
    el.focus();
    el.textContent = spec.value;
  } else {
    return spec.selector + " is not a fillable field";
  }
  el.dispatchEvent(new Event("input", {bubbles: true}));
  el.dispatchEvent(new Event("change", {bubbles: true}));
  return "";
})(%s)`

func (m *Manager) executeFillForm(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[fillFormArgs](argsJSON)
	if err != nil {
		return "", err
	}
	if len(args.Fields) == 0 {
		return "error: fields is required and must not be empty", nil
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	var problems []string
	filled := 0
	for _, f := range args.Fields {
		sel := strings.TrimSpace(f.Selector)
		if sel == "" {
			problems = append(problems, "a field has an empty selector")
			continue
		}
		spec, _ := json.Marshal(map[string]string{"selector": sel, "value": f.Value})
		var problem string
		if err := b.run(chromedp.Evaluate(fmt.Sprintf(fillFieldScript, string(spec)), &problem)); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", sel, err))
			continue
		}
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		filled++
	}

	desc := fmt.Sprintf("filled %d of %d field(s)", filled, len(args.Fields))
	if len(problems) > 0 {
		desc += "; problems: " + strings.Join(problems, "; ")
	}
	return finishAction(b, env, desc)
}
