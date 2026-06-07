package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestLoadSigners(t *testing.T) {
	t.Run("empty dir returns nil", func(t *testing.T) {
		dir := t.TempDir()
		signers := LoadSigners(dir)
		if len(signers) != 0 {
			t.Errorf("LoadSigners(empty dir) = %d signers, want 0", len(signers))
		}
	})

	t.Run("blank dir returns nil", func(t *testing.T) {
		signers := LoadSigners("")
		if signers != nil {
			t.Errorf("LoadSigners(\"\") = %v, want nil", signers)
		}
	})

	t.Run("non-key files ignored", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("not a key"), 0600); err != nil {
			t.Fatal(err)
		}
		signers := LoadSigners(dir)
		if len(signers) != 0 {
			t.Errorf("LoadSigners(bad key file) = %d signers, want 0", len(signers))
		}
	})

	t.Run("valid ed25519 key is loaded", func(t *testing.T) {
		dir := t.TempDir()
		keyPath := filepath.Join(dir, "id_ed25519")
		pemBytes := generateTestED25519Key(t)
		if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
			t.Fatal(err)
		}
		signers := LoadSigners(dir)
		if len(signers) != 1 {
			t.Errorf("LoadSigners(valid ed25519) = %d signers, want 1", len(signers))
		}
	})
}

// generateTestED25519Key generates a fresh unencrypted ed25519 private key in PEM format.
func generateTestED25519Key(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	pemBlock, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return pem.EncodeToMemory(pemBlock)
}
