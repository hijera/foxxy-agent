//go:build browser

package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// RouteTool mocks network responses and toggles offline emulation. It is how the
// agent tests a frontend against an API state the real backend will not produce
// on demand: an error, an empty list, a slow feature flag.
func (m *Manager) RouteTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_route",
			Description: "Control the interactive browser's network. action=mock serves a canned response for every request whose URL " +
				"contains url_substring (status, content_type, body); action=unroute removes mocks (one by url_substring, or all); " +
				"action=list shows active mocks; action=offline / action=online toggle offline emulation. Mocks apply to all tabs " +
				"and to future requests only.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"description": "What to change.",
						"enum":        []interface{}{"mock", "unroute", "list", "offline", "online"},
					},
					"url_substring": map[string]interface{}{
						"type":        "string",
						"description": "Requests whose URL contains this are mocked (action=mock) or un-mocked (action=unroute).",
					},
					"status": map[string]interface{}{
						"type":        "integer",
						"description": "HTTP status for the mocked response (default 200).",
					},
					"content_type": map[string]interface{}{
						"type":        "string",
						"description": "Content-Type for the mocked response (default application/json).",
					},
					"body": map[string]interface{}{
						"type":        "string",
						"description": "Body for the mocked response (may be empty).",
					},
				},
				"required": []interface{}{"action"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeRoute,
	}
}

type routeArgs struct {
	Action       string `json:"action"`
	URLSubstring string `json:"url_substring"`
	Status       int    `json:"status"`
	ContentType  string `json:"content_type"`
	Body         string `json:"body"`
}

func (m *Manager) executeRoute(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[routeArgs](argsJSON)
	if err != nil {
		return "", err
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "mock":
		return routeMockAdd(b, args)
	case "unroute":
		return routeUnroute(b, args.URLSubstring)
	case "list":
		return routeList(b), nil
	case "offline":
		return routeOffline(b, true)
	case "online":
		return routeOffline(b, false)
	default:
		return fmt.Sprintf("error: action must be one of mock, unroute, list, offline, online (got %q)", args.Action), nil
	}
}

func routeMockAdd(b *Browser, args routeArgs) (string, error) {
	substr := strings.TrimSpace(args.URLSubstring)
	if substr == "" {
		return "error: url_substring is required for action=mock", nil
	}
	status := args.Status
	if status == 0 {
		status = 200
	}
	if status < 100 || status > 599 {
		return fmt.Sprintf("error: status must be a valid HTTP status (got %d)", status), nil
	}
	contentType := strings.TrimSpace(args.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}

	b.mu.Lock()
	// Replace an existing mock for the same pattern rather than stacking two.
	replaced := false
	for i := range b.mocks {
		if b.mocks[i].substr == substr {
			b.mocks[i] = routeMock{substr: substr, status: status, contentType: contentType, body: args.Body}
			replaced = true
			break
		}
	}
	if !replaced {
		b.mocks = append(b.mocks, routeMock{substr: substr, status: status, contentType: contentType, body: args.Body})
	}
	needEnable := !b.fetchOn
	b.fetchOn = true
	b.mu.Unlock()

	if needEnable {
		if err := b.forEachTab(func(ctx context.Context) error { return fetch.Enable().Do(ctx) }); err != nil {
			return fmt.Sprintf("error: enable request interception: %v", err), nil
		}
	}
	verb := "mocked"
	if replaced {
		verb = "re-mocked"
	}
	return fmt.Sprintf("%s requests containing %q -> %d %s (%d bytes body); applies to future requests", verb, substr, status, contentType, len(args.Body)), nil
}

func routeUnroute(b *Browser, substr string) (string, error) {
	substr = strings.TrimSpace(substr)
	b.mu.Lock()
	before := len(b.mocks)
	if substr == "" {
		b.mocks = nil
	} else {
		kept := b.mocks[:0]
		for _, mk := range b.mocks {
			if mk.substr != substr {
				kept = append(kept, mk)
			}
		}
		b.mocks = kept
	}
	removed := before - len(b.mocks)
	needDisable := b.fetchOn && len(b.mocks) == 0
	if needDisable {
		b.fetchOn = false
	}
	b.mu.Unlock()

	if needDisable {
		if err := b.forEachTab(func(ctx context.Context) error { return fetch.Disable().Do(ctx) }); err != nil {
			return fmt.Sprintf("removed %d mock(s), but disabling interception failed: %v", removed, err), nil
		}
	}
	if removed == 0 {
		if substr == "" {
			return "no mocks were active", nil
		}
		return fmt.Sprintf("no mock matches %q (use action=list)", substr), nil
	}
	return fmt.Sprintf("removed %d mock(s)", removed), nil
}

func routeList(b *Browser) string {
	b.mu.Lock()
	mocks := append([]routeMock(nil), b.mocks...)
	offline := b.offline
	b.mu.Unlock()
	var sb strings.Builder
	if len(mocks) == 0 {
		sb.WriteString("no mocks active")
	} else {
		fmt.Fprintf(&sb, "%d mock(s):\n", len(mocks))
		for _, mk := range mocks {
			fmt.Fprintf(&sb, "  contains %q -> %d %s (%d bytes body)\n", mk.substr, mk.status, mk.contentType, len(mk.body))
		}
	}
	if offline {
		sb.WriteString("\noffline emulation: on")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func routeOffline(b *Browser, offline bool) (string, error) {
	b.mu.Lock()
	b.offline = offline
	b.mu.Unlock()
	err := b.forEachTab(func(ctx context.Context) error {
		return network.EmulateNetworkConditions(offline, 0, -1, -1).Do(ctx)
	})
	if err != nil {
		return fmt.Sprintf("error: set offline=%v: %v", offline, err), nil
	}
	if offline {
		return "network is now emulated offline (all tabs); action=online restores it", nil
	}
	return "network emulation restored to online", nil
}
