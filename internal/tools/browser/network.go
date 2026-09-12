//go:build browser

package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/network"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// networkListDefault / networkListMax bound the request listing.
const (
	networkListDefault = 30
	networkListMax     = 100
)

// networkBodyLimit keeps a response body from flooding the context.
const networkBodyLimit = 8 * 1024

// NetworkTool reads the request journal the session collects on every tab. The
// page log only reports failures; this answers the other question — what did the
// app actually request, with what status, and what came back.
func (m *Manager) NetworkTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_network",
			Description: "Inspect the interactive browser's network activity. what=requests lists recent requests (method, status, URL, " +
				"type, size, and an id), optionally filtered by a URL substring. what=body returns the response body for an id from the " +
				"list (truncated). Takes no screenshot and does not touch the page.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"what": map[string]interface{}{
						"type":        "string",
						"description": "requests to list, body to fetch one response body.",
						"enum":        []interface{}{"requests", "body"},
					},
					"filter": map[string]interface{}{
						"type":        "string",
						"description": "Optional URL substring to filter the listing (what=requests).",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": fmt.Sprintf("Max requests to list, newest last (default %d, max %d).", networkListDefault, networkListMax),
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Request id from the listing (what=body).",
					},
				},
				"required": []interface{}{"what"},
			},
		},
		RequiresPermission: false,
		Execute:            m.executeNetwork,
	}
}

type networkArgs struct {
	What   string `json:"what"`
	Filter string `json:"filter"`
	Limit  int    `json:"limit"`
	ID     string `json:"id"`
}

func (m *Manager) executeNetwork(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[networkArgs](argsJSON)
	if err != nil {
		return "", err
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	switch strings.ToLower(strings.TrimSpace(args.What)) {
	case "requests":
		return listRequests(b, args.Filter, args.Limit), nil
	case "body":
		return fetchBody(b, args.ID), nil
	default:
		return fmt.Sprintf("error: what must be requests or body (got %q)", args.What), nil
	}
}

func listRequests(b *Browser, filter string, limit int) string {
	if limit <= 0 {
		limit = networkListDefault
	}
	if limit > networkListMax {
		limit = networkListMax
	}
	filter = strings.TrimSpace(filter)

	entries := b.requestLog()
	matched := make([]netEntry, 0, len(entries))
	for _, e := range entries {
		if filter != "" && !strings.Contains(e.url, filter) {
			continue
		}
		matched = append(matched, e)
	}
	total := len(matched)
	if total == 0 {
		if filter != "" {
			return fmt.Sprintf("no requests match %q (%d observed in total)", filter, len(entries))
		}
		return "no requests observed yet"
	}
	if len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d request(s)", total)
	if len(matched) < total {
		fmt.Fprintf(&sb, " (newest %d shown)", len(matched))
	}
	sb.WriteString(":\n")
	for _, e := range matched {
		status := "pending"
		if e.status > 0 {
			status = fmt.Sprintf("%d", e.status)
		}
		fmt.Fprintf(&sb, "%s %s %s", e.method, status, e.url)
		var extras []string
		if e.rtype != "" {
			extras = append(extras, strings.ToLower(e.rtype))
		}
		if e.mime != "" {
			extras = append(extras, e.mime)
		}
		if e.size > 0 {
			extras = append(extras, humanBytes(int64(e.size)))
		}
		if len(extras) > 0 {
			fmt.Fprintf(&sb, " (%s)", strings.Join(extras, ", "))
		}
		fmt.Fprintf(&sb, "  id=%s\n", e.id)
	}
	sb.WriteString("(pass an id to what=body for the response body)")
	return sb.String()
}

// fetchBody pulls a response body over CDP. The request may belong to any tab,
// and CDP only serves the body from the session that owns it, so every tab is
// tried until one answers.
func fetchBody(b *Browser, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "error: id is required for what=body (take it from what=requests)"
	}
	var entry *netEntry
	entries := b.requestLog()
	for i := range entries {
		if string(entries[i].id) == id {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return fmt.Sprintf("error: no observed request has id %q (use what=requests)", id)
	}

	var body []byte
	err := b.forEachTab(func(ctx context.Context) error {
		if body != nil {
			return nil
		}
		got, err := network.GetResponseBody(network.RequestID(id)).Do(ctx)
		if err == nil {
			body = got
		}
		// A miss on one tab is expected; the aggregate outcome is checked below.
		return nil
	})
	_ = err
	if body == nil {
		return fmt.Sprintf("error: response body for %s is not available (the browser may have evicted it)", entry.url)
	}

	truncated := false
	if len(body) > networkBodyLimit {
		body = body[:networkBodyLimit]
		truncated = true
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "body of %s %s (%s):\n", entry.method, entry.url, entry.mime)
	sb.Write(body)
	if truncated {
		fmt.Fprintf(&sb, "\n[truncated at %d bytes]", networkBodyLimit)
	}
	return sb.String()
}
