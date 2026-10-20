//go:build browser

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// waitPollInterval is how often the condition is re-checked. Fine-grained enough
// that a settled SPA is reported quickly, coarse enough not to hammer the page.
const waitPollInterval = 200 * time.Millisecond

// waitDefaultSeconds / waitMaxSeconds bound the wait so a wrong expectation
// cannot stall the agent loop for minutes.
const (
	waitDefaultSeconds = 15
	waitMaxSeconds     = 120
)

// WaitForTool blocks until the page reaches an expected state. It exists so the
// model does not have to poll with read_page/screenshot after every action that
// triggers asynchronous work.
func (m *Manager) WaitForTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_wait_for",
			Description: "Wait until the interactive browser page reaches a state: text appears, text disappears, or an element " +
				"matching a CSS selector becomes visible (conditions are combined). With only seconds set it just sleeps. " +
				"Use after an action that triggers loading instead of polling with read_page. Returns the page state and a screenshot.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "Wait until this text is present on the page.",
					},
					"gone_text": map[string]interface{}{
						"type":        "string",
						"description": "Wait until this text is no longer present on the page (e.g. a spinner label).",
					},
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "Wait until an element matching this CSS selector exists and is rendered.",
					},
					"seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum seconds to wait (default 15, max 120). With no condition given, sleep this long.",
					},
				},
			},
		},
		RequiresPermission: false,
		Execute:            m.executeWaitFor,
	}
}

type waitForArgs struct {
	Text     string `json:"text"`
	GoneText string `json:"gone_text"`
	Selector string `json:"selector"`
	Seconds  int    `json:"seconds"`
}

// waitConditionScript reports whether every requested condition currently holds.
const waitConditionScript = `(function(spec){
  var body = document.body ? document.body.innerText : "";
  if (spec.text && body.indexOf(spec.text) < 0) return false;
  if (spec.gone && body.indexOf(spec.gone) >= 0) return false;
  if (spec.selector) {
    var el = document.querySelector(spec.selector);
    if (!el || el.getClientRects().length === 0) return false;
  }
  return true;
})(%s)`

func (m *Manager) executeWaitFor(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[waitForArgs](argsJSON)
	if err != nil {
		return "", err
	}
	args.Text = strings.TrimSpace(args.Text)
	args.GoneText = strings.TrimSpace(args.GoneText)
	args.Selector = strings.TrimSpace(args.Selector)

	seconds := args.Seconds
	if seconds <= 0 {
		seconds = waitDefaultSeconds
	}
	if seconds > waitMaxSeconds {
		seconds = waitMaxSeconds
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	hasCondition := args.Text != "" || args.GoneText != "" || args.Selector != ""
	if !hasCondition {
		// A plain sleep: bounded tighter, because nothing can cut it short.
		if seconds > 30 {
			seconds = 30
		}
		time.Sleep(time.Duration(seconds) * time.Second)
		return finishAction(b, env, fmt.Sprintf("waited %ds", seconds))
	}

	spec, _ := json.Marshal(map[string]string{
		"text":     args.Text,
		"gone":     args.GoneText,
		"selector": args.Selector,
	})
	script := fmt.Sprintf(waitConditionScript, string(spec))

	start := time.Now()
	deadline := start.Add(time.Duration(seconds) * time.Second)
	for {
		var ok bool
		if err := b.run(chromedp.Evaluate(script, &ok)); err != nil {
			return fmt.Sprintf("error: wait_for: %v", err), nil
		}
		if ok {
			return finishAction(b, env, fmt.Sprintf("condition met after %dms", time.Since(start).Milliseconds()))
		}
		if time.Now().After(deadline) {
			return fmt.Sprintf("error: wait_for: condition not met within %ds (text=%q gone_text=%q selector=%q)",
				seconds, args.Text, args.GoneText, args.Selector), nil
		}
		time.Sleep(waitPollInterval)
	}
}
