package brand

import (
	"os"
	"path/filepath"
	"testing"
)

// repoRoot walks up from the test's directory to the checkout root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repository root (no go.mod up the tree)")
		}
		dir = parent
	}
}

// TestBrandAssetsAreInSyncWithTheirSources is the guard that the social preview
// needed and did not have: it fails when a brand vector is edited and the files
// exported from it are not.
func TestBrandAssetsAreInSyncWithTheirSources(t *testing.T) {
	root := repoRoot(t)
	lock, err := Load(root)
	if err != nil {
		t.Fatalf("load %s: %v", LockPath, err)
	}
	if len(lock.Sources) == 0 || len(lock.Outputs) == 0 {
		t.Fatalf("%s records %d sources and %d outputs; expected both to be populated", LockPath, len(lock.Sources), len(lock.Outputs))
	}
	for _, problem := range lock.Verify(root) {
		t.Errorf("%s; re-run `make brand` and commit the result", problem)
	}
}

func TestVerifyReportsAChangedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "docs", "assets", "mark.svg")
	if err := os.WriteFile(source, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := Hash(root, "docs/assets/mark.svg")
	if err != nil {
		t.Fatal(err)
	}

	lock := &Lock{
		Sources: map[string]string{"docs/assets/mark.svg": hash},
		Outputs: map[string]Output{},
	}
	if problems := lock.Verify(root); len(problems) != 0 {
		t.Fatalf("an untouched source reported %v, want nothing", problems)
	}

	if err := os.WriteFile(source, []byte("<svg><!-- edited --></svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	problems := lock.Verify(root)
	if len(problems) != 1 {
		t.Fatalf("an edited source reported %v, want exactly one problem", problems)
	}
}

func TestVerifyReportsAnOutputWithAnUnknownSource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "docs", "assets", "favicon-32.png")
	if err := os.WriteFile(out, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := Hash(root, "docs/assets/favicon-32.png")
	if err != nil {
		t.Fatal(err)
	}

	lock := &Lock{
		Sources: map[string]string{},
		Outputs: map[string]Output{
			"docs/assets/favicon-32.png": {SHA256: hash, From: []string{"docs/assets/gone.svg"}},
		},
	}
	problems := lock.Verify(root)
	if len(problems) != 1 {
		t.Fatalf("got %v, want one complaint about the unrecorded source", problems)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := &Lock{
		Sources: map[string]string{"docs/assets/mark.svg": "abc"},
		Outputs: map[string]Output{"docs/assets/favicon.ico": {SHA256: "def", From: []string{"docs/assets/mark.svg"}}},
	}
	if err := want.Save(root); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Comment != Comment {
		t.Errorf("comment = %q, want the standard note", got.Comment)
	}
	if got.Sources["docs/assets/mark.svg"] != "abc" || got.Outputs["docs/assets/favicon.ico"].SHA256 != "def" {
		t.Errorf("round trip lost data: %+v", got)
	}
}

// TestHashIgnoresLineEndingsForTextSources guards the lock against the one
// difference git itself introduces: this checkout is CRLF on Windows and LF on
// the Linux runner, so a lock written on one machine has to verify on the other.
func TestHashIgnoresLineEndingsForTextSources(t *testing.T) {
	root := t.TempDir()
	lf := filepath.Join(root, "lf.svg")
	crlf := filepath.Join(root, "crlf.svg")
	if err := os.WriteFile(lf, []byte("<svg>\n  <path/>\n</svg>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crlf, []byte("<svg>\r\n  <path/>\r\n</svg>\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Hash(root, "lf.svg")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Hash(root, "crlf.svg")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("the same SVG hashed %s with LF and %s with CRLF; the lock would fail on the other platform", a[:12], b[:12])
	}
}

// TestHashKeepsRasterBytesExact is the other half: a PNG is binary, git never
// rewrites it, and a CR inside one is content rather than a line ending.
func TestHashKeepsRasterBytesExact(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.png"), []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.png"), []byte("\x89PNG\n\x1a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := Hash(root, "a.png")
	b, _ := Hash(root, "b.png")
	if a == b {
		t.Error("two different PNG byte strings hashed the same; rasters must not be normalised")
	}
}
