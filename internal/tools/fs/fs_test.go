package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/tooling"
	"golang.org/x/text/encoding/charmap"
)

func windows1251Bytes(t *testing.T, value string) []byte {
	t.Helper()
	encoded, err := charmap.Windows1251.NewEncoder().Bytes([]byte(value))
	if err != nil {
		t.Fatalf("encode Windows-1251 fixture: %v", err)
	}
	return encoded
}

func TestReadDecodesWindows1251(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.txt")
	if err := os.WriteFile(path, windows1251Bytes(t, "Привет, мир!\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := executeRead(context.Background(), `{"path":"legacy.txt"}`, &tooling.Env{CWD: root})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "Привет, мир!\n" {
		t.Fatalf("read returned %q", out)
	}
}

func TestReadOffsetBeyondEndReturnsError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "short.txt")
	if err := os.WriteFile(path, []byte("first\nsecond\nthird"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := executeRead(context.Background(), `{"path":"short.txt","offset":1200,"limit":200}`, &tooling.Env{CWD: root})
	if err == nil {
		t.Fatal("expected an out-of-range offset error")
	}
	if !strings.Contains(err.Error(), "offset 1200 is beyond end of file (3 lines)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSliceLinesBoundaryCases(t *testing.T) {
	tests := []struct {
		name    string
		content string
		start   int
		end     int
		want    string
		wantErr string
	}{
		{
			name:    "offset zero is normalized to the first line",
			content: "first\nsecond\nthird",
			start:   0,
			end:     1,
			want:    "first",
		},
		{
			name:    "offset at the final line is valid",
			content: "first\nsecond\nthird",
			start:   3,
			end:     3,
			want:    "third",
		},
		{
			name:    "limit extending beyond EOF is clipped",
			content: "first\nsecond\nthird",
			start:   2,
			end:     200,
			want:    "second\nthird",
		},
		{
			name:    "trailing newline is preserved without creating a phantom line",
			content: "first\nsecond\nthird\n",
			start:   3,
			end:     200,
			want:    "third\n",
		},
		{
			name:    "offset after a trailing newline reports the real line count",
			content: "first\nsecond\nthird\n",
			start:   4,
			end:     200,
			wantErr: "offset 4 is beyond end of file (3 lines)",
		},
		{
			name:    "offset into an empty file reports zero lines",
			content: "",
			start:   1,
			end:     1,
			wantErr: "offset 1 is beyond end of file (0 lines)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sliceLines(tt.content, tt.start, tt.end)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("sliceLines: %v", err)
			}
			if got != tt.want {
				t.Fatalf("result = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEditPreservesWindows1251(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.txt")
	if err := os.WriteFile(path, windows1251Bytes(t, "старый текст\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := executeEdit(context.Background(), `{"path":"legacy.txt","oldString":"старый","newString":"новый"}`, &tooling.Env{CWD: root})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := windows1251Bytes(t, "новый текст\n")
	if string(got) != string(want) {
		t.Fatalf("bytes = %v, want Windows-1251 %v", got, want)
	}
}

func TestWritePreservesExistingWindows1251(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.txt")
	if err := os.WriteFile(path, windows1251Bytes(t, "исходный\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := executeWrite(context.Background(), `{"path":"legacy.txt","content":"перезаписан\n"}`, &tooling.Env{CWD: root})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := windows1251Bytes(t, "перезаписан\n")
	if string(got) != string(want) {
		t.Fatalf("bytes = %v, want Windows-1251 %v", got, want)
	}
}

func TestApplyPatchPreservesWindows1251(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.txt")
	if err := os.WriteFile(path, windows1251Bytes(t, "первая\nвторая\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, err := json.Marshal(map[string]string{
		"path":  "legacy.txt",
		"patch": "@@ -2,1 +2,1 @@\n-вторая\n+новая\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = executeApplyPatch(context.Background(), string(args), &tooling.Env{CWD: root})
	if err != nil {
		t.Fatalf("apply_patch: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := windows1251Bytes(t, "первая\nновая")
	if string(got) != string(want) {
		t.Fatalf("bytes = %v, want Windows-1251 %v", got, want)
	}
}

func TestEditPreviewPreservesWindows1251(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.txt")
	before := windows1251Bytes(t, "до")
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatal(err)
	}

	_, gotBefore, after, ok, err := EditPreview("edit", `{"path":"legacy.txt","oldString":"до","newString":"после"}`, root)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !ok || string(gotBefore) != string(before) {
		t.Fatalf("preview before mismatch: ok=%v bytes=%v", ok, gotBefore)
	}
	wantAfter := windows1251Bytes(t, "после")
	if string(after) != string(wantAfter) {
		t.Fatalf("after = %v, want Windows-1251 %v", after, wantAfter)
	}
}

// --- patch.go: unified diff / v4a patch ------------------------------------

func TestApplyUnifiedDiff_hunkZeroOriginDoesNotPanic(t *testing.T) {
	t.Parallel()
	original := ""
	diff := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -0,0 +1,2 @@",
		"+line one",
		"+line two",
	}, "\n")

	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("applyUnifiedDiff: %v", err)
	}
	want := "line one\nline two"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyUnifiedDiff_replaceMiddleLine(t *testing.T) {
	t.Parallel()
	original := "line1\nline2\nline3\n"
	diff := "@@ -2,1 +2,1 @@\n-line2\n+newline2\n"

	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("applyUnifiedDiff: %v", err)
	}
	want := "line1\nnewline2\nline3"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyV4APatch_codexEnvelope(t *testing.T) {
	t.Parallel()
	original := strings.Join([]string{
		"import splunklib.client as client",
		"import splunklib.results as results",
		"",
		"def splunk_search():",
		"    pass",
	}, "\n")
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: project/src/api/auto_uw_api.py",
		"@@",
		"-import splunklib.client as client",
		"-import splunklib.results as results",
		"+try:",
		"+    import splunklib.client as client  # type: ignore",
		"+    import splunklib.results as results  # type: ignore",
		"+except Exception:",
		"+    client = None",
		"+    results = None",
		"*** End Patch",
	}, "\n")

	got, err := applyPatch(original, patch)
	if err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if !strings.Contains(got, "try:") || !strings.Contains(got, "splunklib.client as client  # type: ignore") {
		t.Fatalf("patch not applied: %q", got)
	}
	if strings.Contains(got, "\nimport splunklib.client as client\n") {
		t.Fatalf("old imports should be removed: %q", got)
	}
}

func TestApplyV4APatch_bareHunkHeader(t *testing.T) {
	t.Parallel()
	original := "alpha\nbeta\ngamma\n"
	patch := "@@\n-beta\n+BETA\n"

	got, err := applyPatch(original, patch)
	if err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	want := "alpha\nBETA\ngamma"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyUnifiedDiff_deleteOutOfRangeReturnsError(t *testing.T) {
	t.Parallel()
	original := "only\n"
	diff := "@@ -5,1 +5,0 @@\n-gone\n"

	_, err := applyUnifiedDiff(original, diff)
	if err == nil {
		t.Fatal("expected error for delete out of range")
	}
}

// --- paths.go / grep.go / glob.go: session-store hiding --------------------

// buildStoreTree lays out a workspace that also contains FoxxyCode's own session store,
// mirroring the real ~/.foxxycode layout where config.yaml sits next to sessions/<id>/.
// It returns the root and the active session dir.
func buildStoreTree(t *testing.T) (root, sessionDir string) {
	t.Helper()
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("provider: moonshot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessionDir = filepath.Join(root, "sessions", "sess_other")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Another session's transcript that mentions both the search term and an unrelated task.
	leak := "moonshot reference and find lyrics for another session\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "messages.json"), []byte(leak), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, sessionDir
}

func TestSessionStoreRootAndIsWithinDir(t *testing.T) {
	if got := sessionStoreRoot(""); got != "" {
		t.Fatalf("empty SessionDir should disable filtering, got %q", got)
	}
	root := sessionStoreRoot("/home/u/.foxxycode/sessions/sess_abc")
	if root != "/home/u/.foxxycode/sessions" {
		t.Fatalf("store root = %q", root)
	}
	if !isWithinDir("/home/u/.foxxycode/sessions/sess_x/messages.json", root) {
		t.Fatal("store file should be within store root")
	}
	if isWithinDir("/home/u/.foxxycode/config.yaml", root) {
		t.Fatal("sibling config must not be treated as within the store")
	}
	if isWithinDir("/home/u/.foxxycode/sessions-archive/x", root) {
		t.Fatal("prefix-similar sibling dir must not match")
	}
}

func TestDropStoreLinesKeepsNonPathLines(t *testing.T) {
	// No SessionDir → unchanged.
	if got := dropStoreLines("a\nb", ""); got != "a\nb" {
		t.Fatalf("expected passthrough, got %q", got)
	}
	in := "/work/sessions/sess_x/messages.json:1:leak\n/work/main.go:2:keep\nno matches found"
	out := dropStoreLines(in, "/work/sessions")
	if strings.Contains(out, "messages.json") {
		t.Fatalf("store line not dropped: %q", out)
	}
	if !strings.Contains(out, "main.go") || !strings.Contains(out, "no matches found") {
		t.Fatalf("non-store lines must be kept: %q", out)
	}
}

func TestGrepHidesSessionStore(t *testing.T) {
	root, sessionDir := buildStoreTree(t)
	env := &tooling.Env{CWD: root, SessionDir: sessionDir}

	args, _ := json.Marshal(map[string]any{"pattern": "moonshot", "path": root})
	out, err := executeGrep(context.Background(), string(args), env)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !strings.Contains(out, "config.yaml") {
		t.Fatalf("expected a real config.yaml match, got: %q", out)
	}
	if strings.Contains(out, "messages.json") || strings.Contains(out, "find lyrics") {
		t.Fatalf("session store leaked into grep results: %q", out)
	}
}

func TestGlobHidesSessionStore(t *testing.T) {
	root, sessionDir := buildStoreTree(t)
	env := &tooling.Env{CWD: root, SessionDir: sessionDir}

	args, _ := json.Marshal(map[string]any{"pattern": "**/*.json", "path": root})
	out, err := executeGlob(context.Background(), string(args), env)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if !strings.Contains(out, "data.json") {
		t.Fatalf("expected the real data.json file, got: %q", out)
	}
	if strings.Contains(out, "messages.json") {
		t.Fatalf("session store leaked into glob results: %q", out)
	}
}
