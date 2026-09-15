package session_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// write creates a file with body and returns its path.
func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadInstructionsSingleFile(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "AGENTS.md", "# Hello\nworld")
	got := session.LoadInstructions(tmp, "", []string{"AGENTS.md"}, nil)
	if want := "# Hello\nworld"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestLoadInstructionsMultipleFilesKeepConfiguredOrder(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "a.md", "first")
	write(t, tmp, "docs/b.md", "second")
	got := session.LoadInstructions(tmp, "", []string{"a.md", "docs/b.md"}, nil)
	want := "first\n\nsecond"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestLoadInstructionsMissingFilesSkipped(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "real.md", "content")
	got := session.LoadInstructions(tmp, "", []string{"missing.md", "real.md", "also-missing.md"}, nil)
	if want := "content"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestLoadInstructionsEmptyFilesSkipped(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "empty.md", "   \n  ")
	write(t, tmp, "content.md", "real")
	got := session.LoadInstructions(tmp, "", []string{"empty.md", "content.md"}, nil)
	if want := "real"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestLoadInstructionsNoneExist(t *testing.T) {
	tmp := t.TempDir()
	if got := session.LoadInstructions(tmp, "", []string{"AGENTS.md"}, nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestLoadInstructionsNoFiles(t *testing.T) {
	if got := session.LoadInstructions("/some/dir", "", nil, nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestLoadInstructionsBlankNameSkipped(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "real.md", "hello")
	got := session.LoadInstructions(tmp, "", []string{"", "  ", "real.md"}, nil)
	if want := "hello"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

// TestLoadInstructionsFOXXYCODEHomeEntry covers a ${FOXXYCODE_HOME} entry an operator
// wrote by hand. The home AGENTS.md itself needs no such entry: it is a
// preamble document (rules.LoadProjectDocs), read whenever it exists.
func TestLoadInstructionsFOXXYCODEHomeEntry(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	write(t, home, "AGENTS.md", "USER_RULES")
	got := session.LoadInstructions(cwd, home, []string{"${FOXXYCODE_HOME}/AGENTS.md"}, nil)
	if got != "USER_RULES" {
		t.Fatalf("content = %q, want the FOXXYCODE_HOME file", got)
	}
}

// TestLoadInstructionsAbsoluteEntry covers the shape the issue asked for:
// instructions.files entries pointing at a shared rules folder outside any
// workspace. filepath.Join used to turn such an entry into <cwd>/<abs>.
func TestLoadInstructionsAbsoluteEntry(t *testing.T) {
	shared := t.TempDir()
	cwd := t.TempDir()
	abs := write(t, shared, "codebase.md", "SHARED_RULES")
	got := session.LoadInstructions(cwd, "", []string{abs}, nil)
	if !strings.Contains(got, "SHARED_RULES") {
		t.Fatalf("content = %q, want the file at the absolute path", got)
	}
}

func TestLoadInstructionsCWDPlaceholder(t *testing.T) {
	cwd := t.TempDir()
	write(t, cwd, "docs/house.md", "HOUSE_STYLE")
	got := session.LoadInstructions(cwd, "", []string{"${CWD}/docs/house.md"}, nil)
	if !strings.Contains(got, "HOUSE_STYLE") {
		t.Fatalf("content = %q, want the ${CWD} entry", got)
	}
}

func TestLoadInstructionsTildeEntry(t *testing.T) {
	userHome := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("USERPROFILE", userHome)
	default:
		t.Setenv("HOME", userHome)
	}
	write(t, userHome, "rules/global.md", "TILDE_RULES")
	got := session.LoadInstructions(t.TempDir(), "", []string{"~/rules/global.md"}, nil)
	if !strings.Contains(got, "TILDE_RULES") {
		t.Fatalf("content = %q, want the ~ entry", got)
	}
}

// TestLoadInstructionsSkipAlreadyLoaded is the dedupe: the project docs
// preamble of the rules block already carries the root AGENTS.md, so the
// instructions block must not send the same bytes a second time.
func TestLoadInstructionsSkipAlreadyLoaded(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	write(t, home, "AGENTS.md", "USER_RULES")
	project := write(t, cwd, "AGENTS.md", "PROJECT_RULES")
	got := session.LoadInstructions(cwd, home, []string{"${FOXXYCODE_HOME}/AGENTS.md", "AGENTS.md"}, []string{project})
	if !strings.Contains(got, "USER_RULES") {
		t.Fatalf("content = %q, want the user file", got)
	}
	if strings.Contains(got, "PROJECT_RULES") {
		t.Fatalf("content = %q, want the project file skipped: the rules block already carries it", got)
	}
}

func TestLoadInstructionsSameFileTwiceLoadedOnce(t *testing.T) {
	cwd := t.TempDir()
	write(t, cwd, "AGENTS.md", "ONCE")
	got := session.LoadInstructions(cwd, "", []string{"AGENTS.md", "./AGENTS.md"}, nil)
	if n := strings.Count(got, "ONCE"); n != 1 {
		t.Fatalf("content = %q, carries the body %d times, want 1", got, n)
	}
}

func TestResolveInstructionFile(t *testing.T) {
	// Real temporary directories, not synthetic "/agent/home": on Windows a
	// rooted path without a volume is not absolute, so a hand-written fixture
	// would resolve differently there than the assertions expect.
	home := t.TempDir()
	cwd := t.TempDir()
	cases := []struct {
		name  string
		entry string
		want  string
	}{
		{"relative anchors at the workspace", "AGENTS.md", filepath.Join(cwd, "AGENTS.md")},
		{"nested relative", "docs/rules.md", filepath.Join(cwd, "docs", "rules.md")},
		{"foxxycode home placeholder", "${FOXXYCODE_HOME}/AGENTS.md", filepath.Join(home, "AGENTS.md")},
		{"cwd placeholder", "${CWD}/AGENTS.md", filepath.Join(cwd, "AGENTS.md")},
		{"absolute stays itself", filepath.Join(home, "shared.md"), filepath.Join(home, "shared.md")},
		{"blank resolves to nothing", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := session.ResolveInstructionFile(tc.entry, cwd, home); got != tc.want {
				t.Fatalf("ResolveInstructionFile(%q) = %q, want %q", tc.entry, got, tc.want)
			}
		})
	}
}

// TestResolveInstructionFileWithoutHome keeps an unresolvable ${FOXXYCODE_HOME}
// entry out of the filesystem instead of reading a literal "${FOXXYCODE_HOME}"
// directory next to the workspace.
func TestResolveInstructionFileWithoutHome(t *testing.T) {
	if got := session.ResolveInstructionFile("${FOXXYCODE_HOME}/AGENTS.md", t.TempDir(), ""); got != "" {
		t.Fatalf("ResolveInstructionFile without a home = %q, want empty", got)
	}
}

// TestLoadInstructionsSymlinkedFileLoadedOnce covers the layout this repository
// itself uses: CLAUDE.md is a symlink to AGENTS.md, so naming both in
// instructions.files must not put the same text in the prompt twice.
func TestLoadInstructionsSymlinkedFileLoadedOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink on Windows needs a privilege the runner may not have")
	}
	cwd := t.TempDir()
	write(t, cwd, "AGENTS.md", "SAME_BYTES")
	if err := os.Symlink(filepath.Join(cwd, "AGENTS.md"), filepath.Join(cwd, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	got := session.LoadInstructions(cwd, "", []string{"AGENTS.md", "CLAUDE.md"}, nil)
	if n := strings.Count(got, "SAME_BYTES"); n != 1 {
		t.Fatalf("content = %q, carries the body %d times, want 1", got, n)
	}

	// The same holds for the skip list: the rules block carries AGENTS.md, so
	// an instructions entry pointing at it through the link is skipped too.
	if got := session.LoadInstructions(cwd, "", []string{"CLAUDE.md"}, []string{filepath.Join(cwd, "AGENTS.md")}); got != "" {
		t.Fatalf("content = %q, want the symlinked project doc skipped", got)
	}
}

func TestLoadInstructionsDoesNotRepeatAFile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("root agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "extra.md"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	inPrompt := filepath.Join(tmp, "AGENTS.md")
	tests := []struct {
		name          string
		files         []string
		alreadyLoaded []string
		want          string
	}{
		{name: "a file already in the prompt is skipped, the rest load", files: []string{"AGENTS.md", "extra.md"}, alreadyLoaded: []string{inPrompt}, want: "extra"},
		{name: "another spelling of a file already in the prompt", files: []string{"./AGENTS.md"}, alreadyLoaded: []string{inPrompt}, want: ""},
		{name: "a file named twice in the list loads once", files: []string{"extra.md", "./extra.md"}, want: "extra"},
		{name: "an already-loaded path that does not exist skips nothing", files: []string{"extra.md"}, alreadyLoaded: []string{filepath.Join(tmp, "gone.md")}, want: "extra"},
		{name: "nothing already loaded", files: []string{"AGENTS.md"}, want: "root agents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := session.LoadInstructions(tmp, "", tt.files, tt.alreadyLoaded); got != tt.want {
				t.Fatalf("LoadInstructions(%q, already loaded %q) = %q, want %q", tt.files, tt.alreadyLoaded, got, tt.want)
			}
		})
	}
}

// The skip compares files on disk, not names: this repository's own CLAUDE.md
// is a symlink to AGENTS.md.
func TestLoadInstructionsSkipsLoadedFileUnderAnotherName(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("root agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	inPrompt := filepath.Join(tmp, "AGENTS.md")

	t.Run("hard link", func(t *testing.T) {
		if err := os.Link(inPrompt, filepath.Join(tmp, "AGENTS.hardlink.md")); err != nil {
			t.Skipf("cannot create a hard link here: %v", err)
		}
		if got := session.LoadInstructions(tmp, "", []string{"AGENTS.hardlink.md"}, []string{inPrompt}); got != "" {
			t.Fatalf("a hard link to a file already in the prompt loaded again: %q", got)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		if err := os.Symlink("AGENTS.md", filepath.Join(tmp, "CLAUDE.md")); err != nil {
			t.Skipf("cannot create a symlink here: %v", err)
		}
		if got := session.LoadInstructions(tmp, "", []string{"CLAUDE.md"}, []string{inPrompt}); got != "" {
			t.Fatalf("a symlink to a file already in the prompt loaded again: %q", got)
		}
	})
	t.Run("differently cased name", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(tmp, "agents.md")); err != nil {
			t.Skip("case-sensitive volume: agents.md is a different file here")
		}
		if got := session.LoadInstructions(tmp, "", []string{"agents.md"}, []string{inPrompt}); got != "" {
			t.Fatalf("a differently cased name of a file already in the prompt loaded again: %q", got)
		}
	})
}
