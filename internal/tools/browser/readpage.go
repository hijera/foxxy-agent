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

// readPageMaxLines bounds the outline. A real application page has thousands of
// elements, and an outline that fills the context is worse than none: the model
// stops reading it and starts guessing again.
const readPageMaxLines = 400

// ReadPageTool renders the page as an indented outline of roles and visible names.
//
// It exists because the alternatives are both poor for a model that cannot see:
// a screenshot it has no way to read, or querySelector calls that need the markup
// to be known in advance. The outline names what is on the page and what can be
// acted on, for a fraction of a screenshot's tokens.
//
// The outline is built by script in the page rather than from the CDP
// accessibility domain on purpose. GetFullAXTree fails outright against this
// repo's pinned cdproto: current Chrome sends enum values the pinned schema does
// not know ("uninteresting"), and the strict unmarshal takes the whole response
// down with it. Reading the DOM has no schema to drift.
func (m *Manager) ReadPageTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_read_page",
			Description: "Read the current page as a text outline: one line per element with its role, visible name, value, and a CSS selector to act on it. " +
				"Takes no screenshot and does not touch the page. Use it to learn what is on a page and how to target it with foxxycode_browser_click or " +
				"foxxycode_browser_fill, and use it instead of a screenshot when the model cannot see images.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"interactive_only": map[string]interface{}{
						"type":        "boolean",
						"description": "Report only elements that can be interacted with (links, buttons, fields). Default false, which reports headings and text as well.",
					},
				},
			},
		},
		RequiresPermission: false,
		Execute:            m.executeReadPage,
	}
}

type readPageArgs struct {
	InteractiveOnly bool `json:"interactive_only"`
}

// readPageScript walks the rendered DOM and returns one entry per element worth
// naming: interactive controls always, plus headings and standalone text. Hidden
// subtrees are skipped, so the outline matches what a person would actually see.
const readPageScript = `(() => {
  const interactiveOnly = %t;
  const MAX = %d;
  const roleOf = (el) => {
    const explicit = el.getAttribute("role");
    if (explicit) return explicit;
    const tag = el.tagName.toLowerCase();
    if (tag === "a") return el.hasAttribute("href") ? "link" : "text";
    if (tag === "button") return "button";
    if (tag === "select") return "combobox";
    if (tag === "textarea") return "textbox";
    if (tag === "input") {
      const t = (el.type || "text").toLowerCase();
      if (t === "checkbox" || t === "radio") return t;
      if (t === "range") return "slider";
      if (t === "submit" || t === "button" || t === "reset") return "button";
      if (t === "hidden") return "";
      return t === "search" ? "searchbox" : "textbox";
    }
    if (/^h[1-6]$/.test(tag)) return "heading";
    if (tag === "img") return "image";
    if (tag === "li") return "listitem";
    if (tag === "th") return "columnheader";
    if (tag === "summary") return "summary";
    return "";
  };
  const INTERACTIVE = new Set(["link","button","combobox","textbox","searchbox","checkbox","radio","slider","tab","menuitem","switch","option"]);
  const nameOf = (el, role) => {
    const aria = el.getAttribute("aria-label");
    if (aria) return aria.trim();
    const labelledBy = el.getAttribute("aria-labelledby");
    if (labelledBy) {
      const t = labelledBy.split(/\s+/).map((id) => (document.getElementById(id) || {}).textContent || "").join(" ").trim();
      if (t) return t;
    }
    if (el.id) {
      const lab = document.querySelector('label[for="' + CSS.escape(el.id) + '"]');
      if (lab && lab.textContent.trim()) return lab.textContent.trim();
    }
    if (role === "textbox" || role === "searchbox" || role === "combobox") {
      return (el.placeholder || el.name || "").trim();
    }
    if (role === "image") return (el.alt || "").trim();
    let own = "";
    for (const n of el.childNodes) if (n.nodeType === 3) own += n.textContent;
    own = own.replace(/\s+/g, " ").trim();
    if (own) return own;
    const all = (el.textContent || "").replace(/\s+/g, " ").trim();
    return el.children.length <= 2 ? all : "";
  };
  const selectorOf = (el) => {
    if (el.id) return "#" + CSS.escape(el.id);
    const nm = el.getAttribute("name");
    if (nm) return el.tagName.toLowerCase() + '[name="' + nm + '"]';
    const testid = el.getAttribute("data-testid");
    if (testid) return '[data-testid="' + testid + '"]';
    const parts = [];
    let node = el;
    while (node && node.nodeType === 1 && parts.length < 4) {
      let part = node.tagName.toLowerCase();
      const parent = node.parentElement;
      if (parent) {
        const same = Array.from(parent.children).filter((c) => c.tagName === node.tagName);
        if (same.length > 1) part += ":nth-of-type(" + (same.indexOf(node) + 1) + ")";
      }
      parts.unshift(part);
      if (node.id) { parts[0] = "#" + CSS.escape(node.id); break; }
      node = node.parentElement;
    }
    return parts.join(" > ");
  };
  const out = [];
  const walk = (el, depth) => {
    if (out.length >= MAX) return;
    const style = window.getComputedStyle(el);
    if (style.display === "none" || style.visibility === "hidden" || el.hidden) return;
    if (el.getAttribute("aria-hidden") === "true") return;
    const role = roleOf(el);
    const interactive = INTERACTIVE.has(role);
    const name = role ? nameOf(el, role) : "";
    const show = interactive || (!interactiveOnly && role && name);
    let next = depth;
    if (show) {
      const entry = { depth: depth, role: role, name: name.slice(0, 120) };
      if (el.value !== undefined && el.value !== null && String(el.value) !== "") entry.value = String(el.value).slice(0, 60);
      if (interactive) entry.selector = selectorOf(el);
      if (el.disabled) entry.disabled = true;
      out.push(entry);
      next = depth + 1;
    }
    for (const child of el.children) walk(child, next);
  };
  walk(document.body, 0);
  return { title: document.title, truncated: out.length >= MAX, nodes: out };
})()`

type readPageNode struct {
	Depth    int    `json:"depth"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Selector string `json:"selector"`
	Disabled bool   `json:"disabled"`
}

type readPageResult struct {
	Title     string         `json:"title"`
	Truncated bool           `json:"truncated"`
	Nodes     []readPageNode `json:"nodes"`
}

func (m *Manager) executeReadPage(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[readPageArgs](argsJSON)
	if err != nil {
		return "", err
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	script := fmt.Sprintf(readPageScript, args.InteractiveOnly, readPageMaxLines)
	var res readPageResult
	if err := b.run(chromedp.Evaluate(script, &res)); err != nil {
		return fmt.Sprintf("error: read page: %v", err), nil
	}

	var sb strings.Builder
	if url := b.currentURL(); url != "" {
		fmt.Fprintf(&sb, "url: %s\n", url)
	}
	if res.Title != "" {
		fmt.Fprintf(&sb, "title: %s\n", res.Title)
	}
	if len(res.Nodes) == 0 {
		sb.WriteString("page has no visible content\n")
	}
	for _, n := range res.Nodes {
		fmt.Fprintf(&sb, "%s%s", strings.Repeat("  ", n.Depth), n.Role)
		if n.Name != "" {
			fmt.Fprintf(&sb, " %q", n.Name)
		}
		if n.Value != "" {
			fmt.Fprintf(&sb, " value=%q", n.Value)
		}
		if n.Disabled {
			sb.WriteString(" [disabled]")
		}
		if n.Selector != "" {
			fmt.Fprintf(&sb, "  selector=%s", n.Selector)
		}
		sb.WriteByte('\n')
	}
	if res.Truncated {
		fmt.Fprintf(&sb, "[outline truncated at %d elements; use interactive_only to narrow it]\n", readPageMaxLines)
	}
	if logs := b.drainPageLog(); len(logs) > 0 {
		sb.WriteString("page log:\n")
		for _, l := range logs {
			fmt.Fprintf(&sb, "  %s\n", l)
		}
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
