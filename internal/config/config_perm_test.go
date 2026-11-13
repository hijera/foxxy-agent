package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// config.yaml can hold provider keys and remote client tokens, so a save must
// leave it readable by its owner only. A backup of it is the same secret.
func TestAtomicWriteConfigYAMLIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := AtomicWriteConfigYAML(path, []byte("agent:\n  max_turns: 3\n")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("config.yaml mode = %o, want 600", got)
	}
}

func TestBackupCurrentIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("agent:\n  max_turns: 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := BackupCurrent(path); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(BackupPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("config backup mode = %o, want 600", got)
	}
}

func TestLegacyMCPMigrationKeepsSecretsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	path := filepath.Join(root, "config.yaml")
	body := "mcp_servers:\n  - name: secret\n    command: server\n    env:\n      - name: TOKEN\n        value: literal-secret\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithPaths(Paths{Home: home, CWD: root, ConfigPath: path}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(GlobalMCPJSONPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("migrated mcp.json mode = %o, want 600", got)
	}
}

// A migration rewrites config.yaml without the keys that moved and keeps a
// timestamped backup of the old bytes. Both hold what the file held -
// provider keys, MCP env values - so neither keeps a loose mode the file
// arrived with: the rewrite tightens it the way every other config save does.
func TestLegacyMigrationTightensPermsOnRewriteAndBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	path := filepath.Join(root, "config.yaml")
	body := "mcp_servers:\n  - name: secret\n    command: server\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithPaths(Paths{Home: home, CWD: root, ConfigPath: path}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o600 {
		t.Fatalf("rewritten config.yaml mode = %o, want 600", got)
	}
	backups, err := filepath.Glob(path + ".bak-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v (err %v), want exactly one", backups, err)
	}
	st, err = os.Stat(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o600 {
		t.Fatalf("migration backup mode = %o, want 600", got)
	}
}

// The directory holding mcp.json or marketplaces.json is created together
// with the private file it gets: 0600 entries in a group-readable directory
// still name the servers and sources this machine talks to. MkdirAll never
// relaxes an existing directory, so only a freshly created one takes the
// owner-only mode; that is the case a first migration into a project hits.
func TestDeclarationDirectoriesAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	t.Run("mcp.json", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "project", ".foxxycode")
		if _, err := UpdateMCPJSONServer(filepath.Join(dir, "mcp.json"), "srv",
			func(MCPJSONServer, bool) (MCPJSONServer, error) {
				return MCPJSONServer{Command: "server"}, nil
			}); err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := st.Mode().Perm(); got != 0o700 {
			t.Fatalf("mcp.json directory mode = %o, want 700", got)
		}
	})
	t.Run("marketplaces.json", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "project", ".foxxycode")
		if err := WriteMarketplacesFile(filepath.Join(dir, "marketplaces.json"),
			MarketplacesFile{Sources: []string{"owner/repo"}}); err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := st.Mode().Perm(); got != 0o700 {
			t.Fatalf("marketplaces.json directory mode = %o, want 700", got)
		}
	})
}
