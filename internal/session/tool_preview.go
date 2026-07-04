package session

import "strings"

// ToolHTTPUserPreviewContentLines is how many real output lines appear before the final "..." row in HTTP/SSE previews (20 visible rows total: 19 data + ellipsis).
const ToolHTTPUserPreviewContentLines = 19

func normalizeLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// PreviewToolOutputForHTTPUser returns the first ToolHTTPUserPreviewContentLines lines; if output is longer, appends one more row "..." so the transcript shows 20 lines before expansion.
func PreviewToolOutputForHTTPUser(full string) (display string, totalLines int, truncated bool) {
	n := ToolHTTPUserPreviewContentLines
	if n < 1 {
		n = 19
	}
	norm := strings.TrimRight(normalizeLines(full), "\n")
	if norm == "" {
		return "", 0, false
	}
	lines := strings.Split(norm, "\n")
	totalLines = len(lines)
	if totalLines <= n {
		return full, totalLines, false
	}
	return strings.Join(lines[:n], "\n") + "\n...", totalLines, true
}

func toolResultPreviewMeta(truncated bool, totalLines, contentLines int) map[string]interface{} {
	if !truncated || totalLines <= contentLines {
		return nil
	}
	return map[string]interface{}{
		"foxxycode": map[string]interface{}{
			"toolResultPreview": map[string]interface{}{
				"truncated":    true,
				"totalLines":   totalLines,
				"previewLines": contentLines,
			},
		},
	}
}

// PreviewToolResultForSessionUpdate trims tool output for ACP/SSE. The model still receives the full string in RoleTool messages.
func PreviewToolResultForSessionUpdate(_ /* toolName */, fullResult string) (display string, meta map[string]interface{}) {
	prev, tl, trunc := PreviewToolOutputForHTTPUser(fullResult)
	return prev, toolResultPreviewMeta(trunc, tl, ToolHTTPUserPreviewContentLines)
}

// PreviewToolResultSnippet matches PreviewToolResultForSessionUpdate (FoxxyCode REST list rows).
func PreviewToolResultSnippet(_ /* toolName */, full string) (snippet string, truncated bool, totalLines int) {
	snip, tl, trunc := PreviewToolOutputForHTTPUser(full)
	return snip, trunc, tl
}
