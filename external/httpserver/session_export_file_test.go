//go:build http

package httpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// writeProbe renders a stub document to disk and registers the cleanup, so each
// test works against a real path without leaving files in the OS temp dir.
func writeProbe(t *testing.T, sessionID, title, ext string, body []byte) string {
	t.Helper()
	path, err := writeExportTempFile(sessionID, exportRendered{
		body:  body,
		ext:   ext,
		title: title,
	})
	if err != nil {
		t.Fatalf("writeExportTempFile: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
	return path
}

func TestWriteExportTempFileNamesFileFromTitle(t *testing.T) {
	path := writeProbe(t, "sess_abc", "Отчёт по задаче", "pdf", []byte("%PDF-1.4"))

	if !filepath.IsAbs(path) {
		t.Fatalf("path %q is not absolute", path)
	}
	if got := filepath.Base(path); got != "Отчёт_по_задаче.pdf" {
		t.Errorf("file name = %q, want %q", got, "Отчёт_по_задаче.pdf")
	}
	// The session id partitions the directory so two chats sharing a title do
	// not overwrite each other.
	if got := filepath.Base(filepath.Dir(path)); got != "sess_abc" {
		t.Errorf("parent directory = %q, want the session id", got)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(body) != "%PDF-1.4" {
		t.Errorf("content = %q, want the rendered body", body)
	}
}

// Re-exporting one chat in one format must replace the file; otherwise the temp
// directory accumulates a copy per click.
func TestWriteExportTempFileOverwritesTheSameExport(t *testing.T) {
	first := writeProbe(t, "sess_abc", "Chat", "json", []byte("{\"v\":1}"))
	second := writeProbe(t, "sess_abc", "Chat", "json", []byte("{\"v\":2}"))

	if first != second {
		t.Fatalf("second export went to %q, want the same path as %q", second, first)
	}
	entries, err := os.ReadDir(filepath.Dir(first))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one file in the export directory, got %d", len(entries))
	}
	body, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "{\"v\":2}" {
		t.Errorf("content = %q, want the newer render", body)
	}
}

func TestWriteExportTempFileSeparatesSessions(t *testing.T) {
	a := writeProbe(t, "sess_one", "Chat", "html", []byte("a"))
	b := writeProbe(t, "sess_two", "Chat", "html", []byte("b"))

	if a == b {
		t.Fatalf("two sessions shared the export path %q", a)
	}
	if filepath.Base(a) != filepath.Base(b) {
		t.Errorf("same title produced different file names: %q vs %q", a, b)
	}
}

// A session title has no length limit, but a path does.
func TestWriteExportTempFileCapsALongTitle(t *testing.T) {
	path := writeProbe(t, "sess_abc", strings.Repeat("длинное_имя_", 40), "json", []byte("{}"))

	stem := strings.TrimSuffix(filepath.Base(path), ".json")
	if n := utf8.RuneCountInString(stem); n > exportFileNameMaxRunes {
		t.Errorf("file stem is %d runes, want at most %d", n, exportFileNameMaxRunes)
	}
	if !utf8.ValidString(stem) {
		t.Error("truncation split a multi-byte rune")
	}
}

// An untitled chat still has to produce a usable name.
func TestWriteExportTempFileFallsBackToTheSessionID(t *testing.T) {
	path := writeProbe(t, "sess_abc", "", "docx", []byte("x"))

	if got := filepath.Base(path); got != "sess_abc.docx" {
		t.Errorf("file name = %q, want the session id", got)
	}
}

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		in, want string
		n        int
	}{
		{"short", "short", 10},
		{"привет", "при", 3},
		// A name that fits is returned untouched; only a cut tidies the seam it
		// leaves behind.
		{"keep_me__", "keep_me__", 10},
		{"trailing__x", "trailing", 10},
		{"", "", 5},
	}
	for _, tc := range cases {
		if got := truncateRunes(tc.in, tc.n); got != tc.want {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
