package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldDropLine(t *testing.T) {
	tests := []struct {
		line           string
		normalizedHost string
		want           bool
	}{
		{"", "192.168.1.1", false},
		{"# comment", "192.168.1.1", false},
		{"192.168.1.1 ssh-ed25519 AAAA", "192.168.1.1", true},
		{"192.168.1.1,192.168.1.2 ssh-ed25519 AAAA", "192.168.1.1", true},
		{"192.168.1.2 ssh-ed25519 AAAA", "192.168.1.1", false},
		// Non-default port: knownhosts.Normalize("192.168.1.1:2222") → "[192.168.1.1]:2222"
		{"[192.168.1.1]:2222 ssh-ed25519 AAAA", "[192.168.1.1]:2222", true},
		{"[192.168.1.1]:2222 ssh-ed25519 AAAA", "192.168.1.1", false},
	}

	for _, tt := range tests {
		got := shouldDropLine(tt.line, tt.normalizedHost)
		if got != tt.want {
			t.Errorf("shouldDropLine(%q, %q) = %v, want %v", tt.line, tt.normalizedHost, got, tt.want)
		}
	}
}

func TestRemoveKnownHost(t *testing.T) {
	t.Run("removes matching line", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "known_hosts")
		content := "192.168.1.1 ssh-ed25519 AAAA\n" +
			"192.168.1.2 ssh-ed25519 BBBB\n"
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		if err := removeKnownHost(path, "192.168.1.1:22"); err != nil {
			t.Fatalf("removeKnownHost() error = %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(got), "192.168.1.1 ssh-ed25519") {
			t.Errorf("removeKnownHost did not remove the entry; got:\n%s", got)
		}
		if !strings.Contains(string(got), "192.168.1.2") {
			t.Errorf("removeKnownHost removed unrelated entry; got:\n%s", got)
		}
	})

	t.Run("missing file is a no-op", func(t *testing.T) {
		if err := removeKnownHost("/tmp/coddy-test-nonexistent-kh", "host"); err != nil {
			t.Errorf("removeKnownHost(missing) error = %v, want nil", err)
		}
	})

	t.Run("comments and blank lines preserved", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "known_hosts")
		content := "# my hosts\n\n192.168.1.1 ssh-ed25519 AAAA\n"
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		if err := removeKnownHost(path, "192.168.1.1:22"); err != nil {
			t.Fatal(err)
		}

		got, _ := os.ReadFile(path)
		if !strings.Contains(string(got), "# my hosts") {
			t.Errorf("removeKnownHost dropped comment line; got:\n%s", got)
		}
	})
}
