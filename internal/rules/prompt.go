package rules

import (
	"strings"
)

// RenderPrompt builds the {{.Rules}} markdown block.
func RenderPrompt(cwd string, stickyAuto, mentioned []*Rule) string {
	return RenderPromptWithDocs(LoadProjectDocs(cwd), stickyAuto, mentioned)
}

// RenderPromptWithDocs builds the {{.Rules}} block around project docs the
// caller already loaded, so the caller knows which files the block carries and
// can keep the other prompt blocks from repeating them.
func RenderPromptWithDocs(docs []ProjectDoc, stickyAuto, mentioned []*Rule) string {
	var parts []string
	for _, d := range docs {
		var b strings.Builder
		b.WriteString("### ")
		b.WriteString(d.Label)
		b.WriteString("\n\n")
		b.WriteString(d.Content)
		parts = append(parts, b.String())
	}
	seen := make(map[string]struct{})
	var dyn []*Rule
	for _, r := range stickyAuto {
		if r == nil {
			continue
		}
		if _, ok := seen[r.ID]; ok {
			continue
		}
		seen[r.ID] = struct{}{}
		dyn = append(dyn, r)
	}
	for _, r := range mentioned {
		if r == nil {
			continue
		}
		if _, ok := seen[r.ID]; ok {
			continue
		}
		seen[r.ID] = struct{}{}
		dyn = append(dyn, r)
	}
	if len(dyn) > 0 {
		var b strings.Builder
		b.WriteString("## Active project rules\n\n")
		for _, r := range dyn {
			head := r.CanonicalName()
			if r.Description != "" {
				b.WriteString("### ")
				b.WriteString(head)
				b.WriteString(" (")
				b.WriteString(r.Description)
				b.WriteString(")\n\n")
			} else {
				b.WriteString("### ")
				b.WriteString(head)
				b.WriteString("\n\n")
			}
			b.WriteString(r.Content)
			b.WriteString("\n\n")
		}
		parts = append(parts, strings.TrimSpace(b.String()))
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
