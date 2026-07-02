package session

import (
	"fmt"
	"strings"

	"github.com/hijera/foxxy-agent/internal/acp"
	"github.com/hijera/foxxy-agent/internal/plans"
)

// HydrateSessionPlanMentions adds resource blocks for @plans/<slug>.plan.md from the session bundle.
func HydrateSessionPlanMentions(sessionDir string, blocks []acp.ContentBlock) ([]acp.ContentBlock, error) {
	covered := make(map[string]struct{})
	for _, b := range blocks {
		if b.Type != "resource" || b.Resource == nil {
			continue
		}
		if strings.TrimSpace(b.Resource.Text) == "" {
			continue
		}
		key := strings.TrimSpace(b.Resource.URI)
		if key != "" {
			covered[key] = struct{}{}
		}
	}
	out := make([]acp.ContentBlock, 0, len(blocks)+2)
	for _, b := range blocks {
		out = append(out, b)
		if b.Type != acp.ContentTypeText && b.Type != "text" {
			continue
		}
		for _, rel := range ExtractAtFilePathsFromText(b.Text) {
			if !plans.IsPlanMention(rel) {
				continue
			}
			key := rel
			if _, ok := covered[key]; ok {
				continue
			}
			covered[key] = struct{}{}
			doc, err := plans.ReadByMention(sessionDir, rel)
			if err != nil {
				return nil, fmt.Errorf("plan mention %s: %w", rel, err)
			}
			out = append(out, acp.ContentBlock{
				Type: "resource",
				Resource: &acp.Resource{
					URI:      rel,
					MimeType: "text/markdown; charset=utf-8",
					Text:     doc.Content,
				},
			})
		}
	}
	return out, nil
}

// ExtractRunPlanSlugFromPromptText returns a slug when the user asks to implement a plan by path or slug.
func ExtractRunPlanSlugFromPromptText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, rel := range ExtractAtFilePathsFromText(text) {
		if !plans.IsPlanMention(rel) {
			continue
		}
		base := rel[strings.LastIndex(rel, "/")+1:]
		return strings.TrimSuffix(base, plans.FileSuffix)
	}
	// "implement the plan auth-refactor" style
	const prefix = "implement the plan "
	if strings.Contains(text, prefix) {
		rest := strings.TrimSpace(text[strings.Index(text, prefix)+len(prefix):])
		if i := strings.IndexAny(rest, " \t\n\r.,;"); i >= 0 {
			rest = rest[:i]
		}
		if err := plans.ValidateSlug(rest); err == nil {
			return rest
		}
	}
	return ""
}

func contentBlocksToPlainText(blocks []acp.ContentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type == acp.ContentTypeText || blk.Type == "text" {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}
