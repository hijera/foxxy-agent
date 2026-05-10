package session

import (
	"strings"
	"testing"
)

func TestPreviewToolOutputForHTTPUser_truncates(t *testing.T) {
	var b strings.Builder
	for i := range 12 {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteByte(byte('a' + i))
	}
	prev, n, trunc := PreviewToolOutputForHTTPUser(b.String(), ToolHTTPUserPreviewLines)
	if !trunc || n != 12 {
		t.Fatalf("trunc=%v n=%d", trunc, n)
	}
	if strings.Count(prev, "\n") != 10 {
		t.Fatalf("want 10 newlines (10 lines + ellipsis line), got %q", prev)
	}
	if !strings.HasSuffix(prev, "\n...") {
		t.Fatalf("want suffix newline + ellipsis, got %q", prev)
	}
}

func TestPreviewToolOutputForHTTPUser_short(t *testing.T) {
	prev, n, trunc := PreviewToolOutputForHTTPUser("a/\nb/", ToolHTTPUserPreviewLines)
	if trunc || n != 2 || prev != "a/\nb/" {
		t.Fatalf("got %q n=%d trunc=%v", prev, n, trunc)
	}
}

func TestPreviewToolResultForSessionUpdate_metaWhenTruncated(t *testing.T) {
	body := strings.Repeat("z\n", 11)
	body = strings.TrimSuffix(body, "\n")
	got, meta := PreviewToolResultForSessionUpdate("any_tool", body)
	if !strings.Contains(got, "...") || meta == nil {
		t.Fatalf("got %q meta=%v", got, meta)
	}
}
