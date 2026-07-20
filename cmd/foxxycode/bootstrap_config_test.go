package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ensureFoxxyCodeHomeLayout is the shared startup hook, so seeding the example
// config must happen there for every entry point (acp, http, desktop, gateway).
func TestEnsureHomeLayoutSeedsExampleConfig(t *testing.T) {
	home := t.TempDir()
	example := filepath.Join(t.TempDir(), "config.example.yaml")
	if err := os.WriteFile(example, []byte("providers: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOXXYCODE_EXAMPLE_CONFIG", example)

	if err := ensureFoxxyCodeHomeLayout(home); err != nil {
		t.Fatalf("ensureFoxxyCodeHomeLayout: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("config.yaml was not seeded: %v", err)
	}
	if string(got) != "providers: []\n" {
		t.Fatalf("seeded content = %q", got)
	}
	for _, name := range []string{"sessions", "skills", "scheduler"} {
		if info, err := os.Stat(filepath.Join(home, name)); err != nil || !info.IsDir() {
			t.Fatalf("%s dir missing: %v", name, err)
		}
	}
}

func TestEnsureHomeLayoutKeepsExistingConfig(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(dest, []byte("providers: [{name: mine}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	example := filepath.Join(t.TempDir(), "config.example.yaml")
	if err := os.WriteFile(example, []byte("providers: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOXXYCODE_EXAMPLE_CONFIG", example)

	if err := ensureFoxxyCodeHomeLayout(home); err != nil {
		t.Fatalf("ensureFoxxyCodeHomeLayout: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "providers: [{name: mine}]\n" {
		t.Fatalf("existing config was overwritten: %q", got)
	}
}

// An empty home means "no state directory configured"; it must stay a no-op.
func TestEnsureHomeLayoutEmptyHomeIsNoop(t *testing.T) {
	if err := ensureFoxxyCodeHomeLayout(""); err != nil {
		t.Fatalf("empty home should be a no-op: %v", err)
	}
}
