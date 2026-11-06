package skills

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// A link named SKILL.md is not a skill file: the copy drops links, so the
// directory would be installed without the file it was found by.
func TestLocateSkillDirsIgnoresALinkedSkillFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere.md")
	if err := os.WriteFile(target, []byte("---\nname: linked\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "linked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "SKILL.md")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	if hits := locateSkillDirs(root); len(hits) != 0 {
		t.Fatalf("a directory whose SKILL.md is a link was taken for a skill: %+v", hits)
	}
}

func TestCheckManifestCloneURL(t *testing.T) {
	tests := []struct {
		name, url, src string
		ok             bool
	}{
		{"https from a remote marketplace", "https://github.com/o/r", "https://example.com/market.json", true},
		{"https from a local marketplace", "https://github.com/o/r", "file:///srv/market", true},
		{"local clone of a local marketplace", "file:///srv/repo", "file:///srv/market", true},
		{"local clone named by a remote manifest", "file:///srv/repo", "https://example.com/market.git", false},
		{"git protocol", "git://internal.example/r.git", "https://example.com/market.git", false},
		{"ssh", "ssh://host/r.git", "https://example.com/market.git", false},
		{"scp style", "git@host:o/r.git", "https://example.com/market.git", false},
		{"bare path", "/local/path", "file:///srv/market", false},
		{"plain http", "http://example.com/r", "https://example.com/market.git", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkManifestCloneURL(tc.url, tc.src)
			if (err == nil) != tc.ok {
				t.Fatalf("checkManifestCloneURL(%q, %q) = %v, want ok=%v", tc.url, tc.src, err, tc.ok)
			}
		})
	}
}

// A plugin path that resolves, through a link in the clone, outside the clone
// must not be read: it would install a host folder as a plugin.
func TestContainedPluginDirRefusesALinkOutOfTheClone(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	outside := filepath.Join(base, "outside", "plugin")
	for _, d := range []string{filepath.Join(repo, "plugins"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(repo, "plugins", "escape")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	if got, err := containedPluginDir(repo, "plugins/escape"); err == nil {
		t.Fatalf("a link out of the clone resolved to %s", got)
	}
	if _, err := containedPluginDir(repo, "../outside/plugin"); err == nil {
		t.Fatal("a lexical escape resolved")
	}
	if _, err := containedPluginDir(repo, "plugins"); err != nil {
		t.Fatalf("a plain directory inside the clone was refused: %v", err)
	}
}

func TestSanitizeSkillNameRefusesAliasingNames(t *testing.T) {
	// A trailing space is trimmed before the check, as it is everywhere a name is read.
	for _, name := range []string{"foo.", "CON", "nul.txt", "com1", "LPT9.md"} {
		if _, err := sanitizeSkillName(name); err == nil {
			t.Errorf("sanitizeSkillName(%q) accepted a name that aliases another path", name)
		}
	}
	for _, name := range []string{"foo", "conductor", "com10", "lpt", "my.skill"} {
		if _, err := sanitizeSkillName(name); err != nil {
			t.Errorf("sanitizeSkillName(%q) refused an ordinary name: %v", name, err)
		}
	}
}

// A backup whose owner is still running may be mid-swap, so recovery must
// leave it; one whose owner is gone is the interrupted install it describes.
func TestRecoverInterruptedInstallsLeavesALiveOwnersBackup(t *testing.T) {
	managed := t.TempDir()
	live := filepath.Join(managed, backupPrefix+"busy-"+strconv.Itoa(os.Getpid()))
	orphan := filepath.Join(managed, backupPrefix+"gone-"+strconv.Itoa(2147483000))
	for _, dir := range []string{live, orphan} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	RecoverInterruptedInstalls(managed)

	if _, err := os.Stat(live); err != nil {
		t.Fatalf("the backup of a running process was touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managed, "gone", "SKILL.md")); err != nil {
		t.Fatalf("the backup of a process that is gone was not restored: %v", err)
	}
}
