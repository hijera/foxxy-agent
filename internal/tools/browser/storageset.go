//go:build browser

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// StorageSetTool writes to the page's stores. inspect(what=storage) reads them;
// this is the other half, so a session can be put into a known state (a feature
// flag, an auth token, a dismissed banner) without hand-written evaluate calls.
func (m *Manager) StorageSetTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_storage_set",
			Description: "Write to the interactive browser's storage: scope=local (localStorage), scope=session (sessionStorage) or " +
				"scope=cookie. action=set writes key/value, action=delete removes a key, action=clear empties the scope. " +
				"Cookies are set for the current page's URL. Reload the page for the app to pick the change up. " +
				"Read the stores with foxxycode_browser_inspect what=storage.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"scope": map[string]interface{}{
						"type":        "string",
						"description": "Which store to write.",
						"enum":        []interface{}{"local", "session", "cookie"},
					},
					"action": map[string]interface{}{
						"type":        "string",
						"description": "What to do.",
						"enum":        []interface{}{"set", "delete", "clear"},
					},
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Key (or cookie name). Required for set and delete.",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "Value to store (action=set).",
					},
				},
				"required": []interface{}{"scope", "action"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeStorageSet,
	}
}

type storageSetArgs struct {
	Scope  string `json:"scope"`
	Action string `json:"action"`
	Key    string `json:"key"`
	Value  string `json:"value"`
}

// webStorageScript performs set/delete/clear on local/sessionStorage. Returns ""
// on success or a problem description.
const webStorageScript = `(function(spec){
  var store = spec.scope === "local" ? window.localStorage : window.sessionStorage;
  try {
    if (spec.action === "set") store.setItem(spec.key, spec.value);
    else if (spec.action === "delete") store.removeItem(spec.key);
    else store.clear();
  } catch (e) { return String(e); }
  return "";
})(%s)`

func (m *Manager) executeStorageSet(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[storageSetArgs](argsJSON)
	if err != nil {
		return "", err
	}
	scope := strings.ToLower(strings.TrimSpace(args.Scope))
	action := strings.ToLower(strings.TrimSpace(args.Action))
	switch scope {
	case "local", "session", "cookie":
	default:
		return fmt.Sprintf("error: scope must be local, session or cookie (got %q)", args.Scope), nil
	}
	switch action {
	case "set", "delete", "clear":
	default:
		return fmt.Sprintf("error: action must be set, delete or clear (got %q)", args.Action), nil
	}
	key := strings.TrimSpace(args.Key)
	if key == "" && action != "clear" {
		return "error: key is required for set and delete", nil
	}

	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	if scope == "cookie" {
		return storageSetCookie(b, action, key, args.Value)
	}

	spec, _ := json.Marshal(map[string]string{"scope": scope, "action": action, "key": key, "value": args.Value})
	var problem string
	if err := b.run(chromedp.Evaluate(fmt.Sprintf(webStorageScript, string(spec)), &problem)); err != nil {
		return fmt.Sprintf("error: %s storage %s: %v", scope, action, err), nil
	}
	if problem != "" {
		return fmt.Sprintf("error: %s storage %s: %s", scope, action, problem), nil
	}
	return storageSetSummary(scope, action, key), nil
}

func storageSetCookie(b *Browser, action, key, value string) (string, error) {
	url := b.currentURL()
	if url == "" && action != "clear" {
		return "error: no page is loaded; navigate first so the cookie has a URL to attach to", nil
	}
	var err error
	switch action {
	case "set":
		err = b.run(chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(key, value).WithURL(url).Do(ctx)
		}))
	case "delete":
		err = b.run(network.DeleteCookies(key).WithURL(url))
	case "clear":
		err = b.run(network.ClearBrowserCookies())
	}
	if err != nil {
		return fmt.Sprintf("error: cookie %s: %v", action, err), nil
	}
	return storageSetSummary("cookie", action, key), nil
}

func storageSetSummary(scope, action, key string) string {
	switch action {
	case "set":
		return fmt.Sprintf("%s storage: set %q; reload the page for the app to pick it up", scope, key)
	case "delete":
		return fmt.Sprintf("%s storage: deleted %q", scope, key)
	default:
		return fmt.Sprintf("%s storage: cleared", scope)
	}
}
