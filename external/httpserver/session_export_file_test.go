//go:build http

package httpserver

import (
	"errors"
	"fmt"
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

// blockPath makes one candidate path impossible to write while leaving the
// directory usable. A directory standing where the file goes reproduces exactly
// the production condition — the path exists and cannot be opened for writing —
// without depending on Windows share modes, which Go's os package does not take.
func blockPath(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("blockPath %s: %v", path, err)
	}
}

// A .docx the user still has open in Word cannot be replaced on Windows. Rather
// than failing the export, the next free name is used.
func TestWriteExportTempFileFallsBackToANumberedName(t *testing.T) {
	first := writeProbe(t, "sess_lock", "Chat", "docx", []byte("one"))
	dir := filepath.Dir(first)
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	blockPath(t, first)

	path, err := writeExportTempFile("sess_lock", exportRendered{
		body: []byte("two"), ext: "docx", title: "Chat",
	})
	if err != nil {
		t.Fatalf("writeExportTempFile: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	if got := filepath.Base(path); got != "Chat_1.docx" {
		t.Fatalf("file name = %q, want %q", got, "Chat_1.docx")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "two" {
		t.Errorf("content = %q, want the new render", body)
	}
}

func TestWriteExportTempFileKeepsCountingUp(t *testing.T) {
	seed := writeProbe(t, "sess_lock2", "Chat", "docx", []byte("x"))
	dir := filepath.Dir(seed)
	if err := os.Remove(seed); err != nil {
		t.Fatal(err)
	}
	blockPath(t, seed)
	blockPath(t, filepath.Join(dir, "Chat_1.docx"))

	path, err := writeExportTempFile("sess_lock2", exportRendered{
		body: []byte("y"), ext: "docx", title: "Chat",
	})
	if err != nil {
		t.Fatalf("writeExportTempFile: %v", err)
	}
	if got := filepath.Base(path); got != "Chat_2.docx" {
		t.Fatalf("file name = %q, want %q", got, "Chat_2.docx")
	}
}

// With every candidate blocked the caller must be able to tell this apart from a
// broken disk, so the panel can say the name is in use.
func TestWriteExportTempFileReportsWhenEveryNameIsTaken(t *testing.T) {
	seed := writeProbe(t, "sess_lock3", "Chat", "docx", []byte("x"))
	dir := filepath.Dir(seed)
	if err := os.Remove(seed); err != nil {
		t.Fatal(err)
	}
	blockPath(t, seed)
	for i := 1; i < exportFileNameMaxAttempts; i++ {
		blockPath(t, filepath.Join(dir, fmt.Sprintf("Chat_%d.docx", i)))
	}

	_, err := writeExportTempFile("sess_lock3", exportRendered{
		body: []byte("y"), ext: "docx", title: "Chat",
	})
	if !errors.Is(err, errExportNameUnavailable) {
		t.Fatalf("error = %v, want errExportNameUnavailable", err)
	}
}

// A free base name must still be reused rather than incremented, so repeated
// exports of one chat do not drift to Chat_1, Chat_2 on their own.
func TestWriteExportTempFileDoesNotNumberAFreeName(t *testing.T) {
	first := writeProbe(t, "sess_free", "Chat", "html", []byte("a"))
	second := writeProbe(t, "sess_free", "Chat", "html", []byte("b"))

	if first != second {
		t.Fatalf("a writable name was incremented: %q then %q", first, second)
	}
	if strings.Contains(filepath.Base(first), "_1") {
		t.Fatalf("unexpected numeric suffix on a free name: %q", first)
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
