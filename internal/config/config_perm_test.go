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
