//go:build browser

package browser

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// InspectTool answers the questions a picture cannot: what the app has stored,
// where the page load spent its time, and how heavy the page has become.
//
// One tool with a subject rather than three tools, because every definition is
// sent on every request and a longer list makes the choice harder for a small
// model. All three are readable through evaluate as well; the value here is that
// the model does not have to know which incantation to write, and that the answer
// comes back in the same shape every time.
func (m *Manager) InspectTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_inspect",
			Description: "Inspect the current page without touching it and without a screenshot. " +
				"what=storage reports localStorage, sessionStorage and cookies (values truncated) — use it when the app behaves differently than expected. " +
				"what=timing reports page load phases and the slowest requests — use it when something is slow. " +
				"what=memory reports the JS heap and DOM size — use it when a page grows heavy over time.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"what": map[string]interface{}{
						"type":        "string",
						"description": "Which subject to report on.",
						"enum":        []interface{}{"storage", "timing", "memory"},
					},
				},
				"required": []interface{}{"what"},
			},
		},
		RequiresPermission: false,
		Execute:            m.executeInspect,
	}
}

type inspectArgs struct {
	What string `json:"what"`
}

// inspectValueLimit keeps a stored credential or a long blob from filling the
// context. Seeing that a key is set, and roughly what it holds, is the point.
const inspectValueLimit = 60

// storageScript reads the three stores a page can hold. httpOnly cookies are
// invisible to script by design; the report says so rather than implying the
// cookie jar is empty.
const storageScript = `(() => {
  const read = (store) => {
    const out = [];
    try {
      for (let i = 0; i < store.length; i++) {
        const k = store.key(i);
        out.push({ key: k, value: String(store.getItem(k) || "") });
      }
    } catch (e) { return [{ key: "(unavailable)", value: String(e) }]; }
    return out;
  };
  const cookies = (document.cookie || "").split(";").map((c) => c.trim()).filter(Boolean).map((c) => {
    const i = c.indexOf("=");
    return i < 0 ? { key: c, value: "" } : { key: c.slice(0, i), value: c.slice(i + 1) };
  });
  return { local: read(window.localStorage), session: read(window.sessionStorage), cookies: cookies };
})()`

// timingScript reads the Navigation Timing entry plus the slowest resources.
const timingScript = `(() => {
  const nav = performance.getEntriesByType("navigation")[0];
  const ms = (v) => (typeof v === "number" && v >= 0 ? Math.round(v) : null);
  const phases = nav ? {
    dns: ms(nav.domainLookupEnd - nav.domainLookupStart),
    connect: ms(nav.connectEnd - nav.connectStart),
    ttfb: ms(nav.responseStart - nav.requestStart),
    response: ms(nav.responseEnd - nav.responseStart),
    dom_content_loaded: ms(nav.domContentLoadedEventEnd),
    load: ms(nav.loadEventEnd || nav.duration)
  } : null;
  const res = performance.getEntriesByType("resource")
    .map((r) => ({ name: r.name, ms: Math.round(r.duration), size: r.transferSize || 0 }))
    .sort((a, b) => b.ms - a.ms)
    .slice(0, 10);
  return { phases: phases, resources: res, resource_count: performance.getEntriesByType("resource").length };
})()`

// memoryScript reports the JS heap where Chrome exposes it, and the DOM size,
// which is always available and usually the more actionable number.
const memoryScript = `(() => {
  const m = performance.memory;
  return {
    heap: m ? { used: m.usedJSHeapSize, total: m.totalJSHeapSize, limit: m.jsHeapSizeLimit } : null,
    dom_nodes: document.getElementsByTagName("*").length,
    listeners_hint: document.querySelectorAll("[onclick]").length
  };
})()`

type storageEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type storageReport struct {
	Local   []storageEntry `json:"local"`
	Session []storageEntry `json:"session"`
	Cookies []storageEntry `json:"cookies"`
}

type timingReport struct {
	Phases    map[string]*int `json:"phases"`
	Resources []struct {
		Name string `json:"name"`
		Ms   int    `json:"ms"`
		Size int    `json:"size"`
	} `json:"resources"`
	ResourceCount int `json:"resource_count"`
}

type memoryReport struct {
	Heap *struct {
		Used  int64 `json:"used"`
		Total int64 `json:"total"`
		Limit int64 `json:"limit"`
	} `json:"heap"`
	DOMNodes      int `json:"dom_nodes"`
	ListenersHint int `json:"listeners_hint"`
}

func (m *Manager) executeInspect(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[inspectArgs](argsJSON)
	if err != nil {
		return "", err
	}
	what := strings.ToLower(strings.TrimSpace(args.What))
	switch what {
	case "storage", "timing", "memory":
	default:
		return fmt.Sprintf("error: what must be one of storage, timing, memory (got %q)", args.What), nil
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	switch what {
	case "storage":
		return inspectStorage(b)
	case "timing":
		return inspectTiming(b)
	default:
		return inspectMemory(b)
	}
}

func inspectStorage(b *Browser) (string, error) {
	var res storageReport
	if err := b.run(chromedp.Evaluate(storageScript, &res)); err != nil {
		return fmt.Sprintf("error: inspect storage: %v", err), nil
	}
	var sb strings.Builder
	writeStore := func(label string, entries []storageEntry) {
		if len(entries) == 0 {
			fmt.Fprintf(&sb, "%s: empty\n", label)
			return
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
		fmt.Fprintf(&sb, "%s (%d):\n", label, len(entries))
		for _, e := range entries {
			fmt.Fprintf(&sb, "  %s = %s\n", e.Key, truncateValue(e.Value))
		}
	}
	writeStore("localStorage", res.Local)
	writeStore("sessionStorage", res.Session)
	writeStore("cookies", res.Cookies)
	sb.WriteString("note: httpOnly cookies are not visible to page script and are not listed here.\n")
	return strings.TrimRight(sb.String(), "\n"), nil
}

func inspectTiming(b *Browser) (string, error) {
	var res timingReport
	if err := b.run(chromedp.Evaluate(timingScript, &res)); err != nil {
		return fmt.Sprintf("error: inspect timing: %v", err), nil
	}
	var sb strings.Builder
	if len(res.Phases) == 0 {
		sb.WriteString("timing: no navigation entry (the page may have been loaded before this session)\n")
	} else {
		sb.WriteString("load phases (ms):\n")
		for _, k := range []string{"dns", "connect", "ttfb", "response", "dom_content_loaded", "load"} {
			if v, ok := res.Phases[k]; ok && v != nil {
				fmt.Fprintf(&sb, "  %s: %d\n", k, *v)
			}
		}
	}
	fmt.Fprintf(&sb, "requests: %d\n", res.ResourceCount)
	if len(res.Resources) > 0 {
		sb.WriteString("slowest:\n")
		for _, r := range res.Resources {
			fmt.Fprintf(&sb, "  %dms %s (%d bytes)\n", r.Ms, truncateRunes(r.Name, 100), r.Size)
		}
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func inspectMemory(b *Browser) (string, error) {
	var res memoryReport
	if err := b.run(chromedp.Evaluate(memoryScript, &res)); err != nil {
		return fmt.Sprintf("error: inspect memory: %v", err), nil
	}
	var sb strings.Builder
	if res.Heap != nil {
		fmt.Fprintf(&sb, "js_heap: used %s of %s (limit %s)\n",
			humanBytes(res.Heap.Used), humanBytes(res.Heap.Total), humanBytes(res.Heap.Limit))
	} else {
		// Chrome only exposes performance.memory in some configurations. Say so
		// rather than printing zeroes that read like a measurement.
		sb.WriteString("js_heap: unavailable (performance.memory is not exposed in this browser configuration)\n")
	}
	fmt.Fprintf(&sb, "dom_nodes: %d\n", res.DOMNodes)
	fmt.Fprintf(&sb, "inline_onclick_handlers: %d\n", res.ListenersHint)
	return strings.TrimRight(sb.String(), "\n"), nil
}

func truncateValue(v string) string {
	v = strings.ReplaceAll(strings.ReplaceAll(v, "\n", " "), "\r", "")
	if len([]rune(v)) <= inspectValueLimit {
		return v
	}
	return truncateRunes(v, inspectValueLimit) + fmt.Sprintf(" (%d chars)", len(v))
}

// truncateRunes cuts on rune boundaries so a multi-byte name is never split.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
