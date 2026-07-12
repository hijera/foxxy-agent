//go:build browser

package browser

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// NavigateTool navigates the session browser to a URL.
func (m *Manager) NavigateTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "foxxycode_browser_navigate",
			Description: "Open a URL in the interactive browser (a real Chrome/Chromium instance). Returns the resolved URL, page console output, and a screenshot the you can see. Use this to load web pages or local dev servers before clicking, filling, or inspecting them.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Absolute http:// or https:// URL to open (localhost is allowed).",
					},
				},
				"required": []string{"url"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeNavigate,
	}
}

type navigateArgs struct {
	URL string `json:"url"`
}

func (m *Manager) executeNavigate(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[navigateArgs](argsJSON)
	if err != nil {
		return "", err
	}
	target, err := validateNavigateURL(args.URL)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	if err := b.run(chromedp.Navigate(target)); err != nil {
		return fmt.Sprintf("error: navigate %s: %v", target, err), nil
	}
	return finishAction(b, env, "navigated to "+target)
}

// validateNavigateURL requires an http(s) URL without embedded credentials.
// Unlike the webfetch SSRF guard it permits localhost/loopback/private hosts,
// because driving a local dev server is the primary use case for the browser tool
// (which is itself opt-in behind the browser build tag and browser.enabled).
func validateNavigateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported url scheme %q (use http or https)", u.Scheme)
	}
	if u.User != nil {
		return "", fmt.Errorf("url must not contain userinfo credentials")
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("url must include a host")
	}
	return u.String(), nil
}

// sessionKey / sessionDir extract the identifiers a tool needs from the Env,
// nil-safe.
func sessionKey(env *tooling.Env) string {
	if env == nil {
		return ""
	}
	return env.SessionID
}

func sessionDir(env *tooling.Env) string {
	if env == nil {
		return ""
	}
	return env.SessionDir
}
