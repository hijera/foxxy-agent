//go:build browser

package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// TabsTool manages browser tabs. Before it existed the session was blind to any
// page opened with target=_blank, and could not compare two pages side by side.
func (m *Manager) TabsTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_tabs",
			Description: "Manage tabs in the interactive browser. action=list numbers the open tabs (URL, title, current). " +
				"action=new opens a tab (optionally at url) and switches to it. action=select / action=close take the tab " +
				"number from the latest list. Pages the site opened itself (target=_blank) appear in the list and can be selected.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"description": "What to do with the tabs.",
						"enum":        []interface{}{"list", "new", "select", "close"},
					},
					"tab": map[string]interface{}{
						"type":        "integer",
						"description": "1-based tab number from action=list (required for select and close).",
					},
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Optional http(s) URL to open in the new tab (action=new).",
					},
				},
				"required": []interface{}{"action"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeTabs,
	}
}

type tabsArgs struct {
	Action string `json:"action"`
	Tab    int    `json:"tab"`
	URL    string `json:"url"`
}

func (m *Manager) executeTabs(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[tabsArgs](argsJSON)
	if err != nil {
		return "", err
	}
	action := strings.ToLower(strings.TrimSpace(args.Action))
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	switch action {
	case "list":
		return b.renderTabList()
	case "new":
		return m.tabsNew(b, args.URL, env)
	case "select":
		return m.tabsSelect(b, args.Tab, env)
	case "close":
		return b.tabsClose(args.Tab)
	default:
		return fmt.Sprintf("error: action must be one of list, new, select, close (got %q)", args.Action), nil
	}
}

func (b *Browser) renderTabList() (string, error) {
	pages, err := b.listPages()
	if err != nil {
		return fmt.Sprintf("error: list tabs: %v", err), nil
	}
	b.mu.Lock()
	current := b.currentID
	b.mu.Unlock()
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d tab(s):\n", len(pages))
	for i, p := range pages {
		marker := " "
		if p.TargetID == current {
			marker = "*"
		}
		title := strings.TrimSpace(p.Title)
		if title != "" {
			title = " — " + title
		}
		fmt.Fprintf(&sb, "%s tab %d: %s%s\n", marker, i+1, p.URL, title)
	}
	sb.WriteString("(* = current; pass the tab number to action=select or action=close)")
	return sb.String(), nil
}

func (m *Manager) tabsNew(b *Browser, rawURL string, env *tooling.Env) (string, error) {
	id, err := b.newTab()
	if err != nil {
		return fmt.Sprintf("error: new tab: %v", err), nil
	}
	if err := b.selectTab(id); err != nil {
		return fmt.Sprintf("error: switch to new tab: %v", err), nil
	}
	desc := "opened a new tab"
	if strings.TrimSpace(rawURL) != "" {
		targetURL, err := validateNavigateURL(rawURL)
		if err != nil {
			return fmt.Sprintf("error: %v", err), nil
		}
		if err := b.run(chromedp.Navigate(targetURL)); err != nil {
			return fmt.Sprintf("error: navigate new tab to %s: %v", targetURL, err), nil
		}
		desc = "opened a new tab at " + targetURL
	}
	return finishAction(b, env, desc)
}

func (m *Manager) tabsSelect(b *Browser, num int, env *tooling.Env) (string, error) {
	pages, err := b.listPages()
	if err != nil {
		return fmt.Sprintf("error: list tabs: %v", err), nil
	}
	if num < 1 || num > len(pages) {
		return fmt.Sprintf("error: tab %d does not exist (there are %d tabs; use action=list)", num, len(pages)), nil
	}
	if err := b.selectTab(pages[num-1].TargetID); err != nil {
		return fmt.Sprintf("error: select tab %d: %v", num, err), nil
	}
	return finishAction(b, env, fmt.Sprintf("switched to tab %d", num))
}

func (b *Browser) tabsClose(num int) (string, error) {
	pages, err := b.listPages()
	if err != nil {
		return fmt.Sprintf("error: list tabs: %v", err), nil
	}
	if len(pages) <= 1 {
		return "error: refusing to close the last tab; use foxxycode_browser_close to end the browser session", nil
	}
	if num < 1 || num > len(pages) {
		return fmt.Sprintf("error: tab %d does not exist (there are %d tabs; use action=list)", num, len(pages)), nil
	}
	if err := b.closeTab(pages[num-1].TargetID); err != nil {
		return fmt.Sprintf("error: close tab %d: %v", num, err), nil
	}
	return fmt.Sprintf("closed tab %d", num), nil
}
