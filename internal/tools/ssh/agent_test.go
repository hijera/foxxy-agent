package ssh

import (
	"os"
	"testing"
)

func TestAgentAuth_NoSocket(t *testing.T) {
	// Ensure SSH_AUTH_SOCK is unset for this test.
	orig := os.Getenv("SSH_AUTH_SOCK")
	if err := os.Unsetenv("SSH_AUTH_SOCK"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("SSH_AUTH_SOCK", orig)
		}
	})

	method, closer, err := agentAuth()
	if err != nil {
		t.Errorf("agentAuth() error = %v, want nil", err)
	}
	if method != nil {
		t.Error("agentAuth() method should be nil when SSH_AUTH_SOCK is not set")
	}
	if closer != nil {
		t.Error("agentAuth() closer should be nil when SSH_AUTH_SOCK is not set")
	}
}

func TestAgentAuth_BadSocket(t *testing.T) {
	// Point SSH_AUTH_SOCK at a path that does not exist.
	orig := os.Getenv("SSH_AUTH_SOCK")
	if err := os.Setenv("SSH_AUTH_SOCK", "/tmp/foxxycode-test-nonexistent.sock"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("SSH_AUTH_SOCK", orig)
		} else {
			_ = os.Unsetenv("SSH_AUTH_SOCK")
		}
	})

	// Should not return an error — unreachable agent is a soft failure.
	method, closer, err := agentAuth()
	if err != nil {
		t.Errorf("agentAuth() error = %v, want nil (soft failure)", err)
	}
	if method != nil {
		t.Error("agentAuth() should return nil method for bad socket")
	}
	if closer != nil {
		t.Error("agentAuth() should return nil closer for bad socket")
	}
}

func TestBuildAuthMethods_NoAgentNoKeys(t *testing.T) {
	orig := os.Getenv("SSH_AUTH_SOCK")
	if err := os.Unsetenv("SSH_AUTH_SOCK"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("SSH_AUTH_SOCK", orig)
		}
	})

	methods, closer := buildAuthMethods(t.TempDir()) // empty dir, no keys
	if closer != nil {
		t.Error("closer should be nil when no agent")
		_ = closer.Close()
	}
	if len(methods) != 0 {
		t.Errorf("buildAuthMethods() = %d methods, want 0", len(methods))
	}
}
