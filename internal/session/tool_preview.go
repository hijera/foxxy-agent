package session

import "strings"

// ToolHTTPUserPreviewLines is max lines streamed to Coddy HTTP (SSE) and in tool-call list previews. Longer tool output loads via GET …/tool-calls/{id} when the user expands.
const ToolHTTPUserPreviewLines = 10

func normalizeLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// PreviewToolOutputForHTTPUser returns the first maxLines lines; if the body has more lines, appends a newline and "...".
func PreviewToolOutputForHTTPUser(full string, maxLines int) (display string, totalLines int, truncated bool) {
	if maxLines < 1 {
		maxLines = ToolHTTPUserPreviewLines
	}
	norm := strings.TrimRight(normalizeLines(full), "\n")
	if norm == "" {
		return "", 0, false
	}
	lines := strings.Split(norm, "\n")
	totalLines = len(lines)
	if totalLines <= maxLines {
		return full, totalLines, false
	}
	return strings.Join(lines[:maxLines], "\n") + "\n...", totalLines, true
}

func toolResultPreviewMeta(truncated bool, totalLines, previewLines int) map[string]interface{} {
	if !truncated || totalLines <= previewLines {
		return nil
	}
	return map[string]interface{}{
		"coddy": map[string]interface{}{
			"toolResultPreview": map[string]interface{}{
				"truncated":    true,
				"totalLines":   totalLines,
				"previewLines": previewLines,
			},
		},
	}
}

// PreviewToolResultForSessionUpdate trims tool output for ACP/SSE (first lines + "..."). The model still receives the full string in RoleTool messages.
func PreviewToolResultForSessionUpdate(_ /* toolName */, fullResult string) (display string, meta map[string]interface{}) {
	prev, tl, trunc := PreviewToolOutputForHTTPUser(fullResult, ToolHTTPUserPreviewLines)
	return prev, toolResultPreviewMeta(trunc, tl, ToolHTTPUserPreviewLines)
}

// PreviewToolResultSnippet matches PreviewToolResultForSessionUpdate (for Coddy REST list rows).
func PreviewToolResultSnippet(_ /* toolName */, full string) (snippet string, truncated bool, totalLines int) {
	snip, tl, trunc := PreviewToolOutputForHTTPUser(full, ToolHTTPUserPreviewLines)
	return snip, trunc, tl
}
