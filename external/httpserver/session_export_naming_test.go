//go:build http

package httpserver

// Content-Disposition naming for the session export download. The rendering
// itself lives in internal/export.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/export"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestExportBaseName(t *testing.T) {
	cases := map[string]string{
		"My Chat": "My_Chat",
		"a/b:c":   "abc",
		"":        "sess_x",
		`x"y|z`:   "xyz",
		"Привет":  "Привет",
	}
	for in, want := range cases {
		if got := exportBaseName(in, "sess_x"); got != want {
			t.Errorf("exportBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestExportContentDisposition pins the RFC 6266 shape: an ASCII-only plain
// filename any client can read, plus an RFC 8187 filename* carrying the real
// (possibly non-Latin) title. A title made only of non-ASCII characters has no
// meaningful ASCII form, so the fallback degrades to the session id.
func TestExportContentDisposition(t *testing.T) {
	cases := []struct {
		title      string
		wantPlain  string
		wantEncSub string
	}{
		{"My Chat", `filename="My_Chat.pdf"`, "filename*=UTF-8''My_Chat.pdf"},
		{"", `filename="sess_x.pdf"`, "filename*=UTF-8''sess_x.pdf"},
		{"Отчёт по задаче", `filename="sess_x.pdf"`, "filename*=UTF-8''%D0%9E%D1%82%D1%87%D1%91%D1%82_%D0%BF%D0%BE_%D0%B7%D0%B0%D0%B4%D0%B0%D1%87%D0%B5.pdf"},
		{"Sprint 3 — обзор", `filename="Sprint_3.pdf"`, ""},
	}
	for _, tc := range cases {
		got := exportContentDisposition(tc.title, "sess_x", "pdf")
		if !strings.HasPrefix(got, "attachment; ") {
			t.Errorf("exportContentDisposition(%q) = %q, missing attachment prefix", tc.title, got)
		}
		if !strings.Contains(got, tc.wantPlain) {
			t.Errorf("exportContentDisposition(%q) = %q, want plain %s", tc.title, got, tc.wantPlain)
		}
		if tc.wantEncSub != "" && !strings.Contains(got, tc.wantEncSub) {
			t.Errorf("exportContentDisposition(%q) = %q, want %s", tc.title, got, tc.wantEncSub)
		}
	}
}

// TestExportContentDispositionCannotBreakTheHeader guards the encoding: a title
// carrying header separators must not introduce new parameters, and control
// characters must never reach the wire.
func TestExportContentDispositionCannotBreakTheHeader(t *testing.T) {
	got := exportContentDisposition("evil; name=oops\r\nX-Injected: 1", "sess_x", "json")
	if strings.Contains(got, "\r") || strings.Contains(got, "\n") {
		t.Fatalf("header value carries a line break: %q", got)
	}
	if strings.Count(got, ";") != 2 {
		t.Fatalf("header gained extra parameters: %q", got)
	}
	marker := "filename*=UTF-8''"
	encoded := got[strings.Index(got, marker)+len(marker):]
	if strings.ContainsAny(encoded, `;,="' `) {
		t.Fatalf("filename* carries raw header separators: %q", encoded)
	}
}

// TestExportContentDispositionFoldsTheIDFallback covers the path where the title
// has no ASCII form and the session id supplies the plain filename: the id comes
// off the request path, so it must be folded rather than trusted.
func TestExportContentDispositionFoldsTheIDFallback(t *testing.T) {
	got := exportContentDisposition("Отчёт", `sess"; evil=1`, "json")
	m := regexp.MustCompile(`filename="([^"]*)"`).FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("no plain filename in %q", got)
	}
	if strings.ContainsAny(m[1], `";=`) {
		t.Fatalf("session id reached the header unchecked: %q", m[1])
	}
	if strings.Count(got, ";") != 2 {
		t.Fatalf("header gained extra parameters: %q", got)
	}
}

func TestHasExportableAssistantAnswer(t *testing.T) {
	if hasExportableAssistantAnswer([]llm.Message{{Role: llm.RoleUser, Content: "hi"}}) {
		t.Fatal("should be false with no assistant message")
	}
	if !hasExportableAssistantAnswer([]llm.Message{{Role: llm.RoleAssistant, Content: "answer"}}) {
		t.Fatal("should be true with assistant content")
	}
	if hasExportableAssistantAnswer([]llm.Message{{Role: llm.RoleAssistant, Content: "   "}}) {
		t.Fatal("should be false with whitespace-only assistant content")
	}
}

// The download accepts every format /export writes, so the panel and the
// command can produce the same file.
func TestExportFormatQueryAcceptsEveryFormat(t *testing.T) {
	for _, token := range []string{"md", "markdown", "html", "json", "jsonl", "pdf", "docx", "PDF"} {
		if _, ok := export.ParseExportFormat(token); !ok {
			t.Errorf("%q should be accepted", token)
		}
	}
	for _, bad := range []string{"rtf", "", "csv"} {
		if _, ok := export.ParseExportFormat(bad); ok {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
