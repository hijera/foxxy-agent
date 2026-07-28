//go:build miniapps

package miniapps

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableRuntimeProvisioningIsPrivateVerifiedAndCached(t *testing.T) {
	var payload bytes.Buffer
	archive := zip.NewWriter(&payload)
	name := "bin/demo-runtime"
	if pathExt := filepath.Ext(os.Args[0]); pathExt == ".exe" {
		name += ".exe"
	}
	item, err := archive.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := item.Write([]byte("portable-runtime")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write(payload.Bytes())
	}))
	defer server.Close()

	sum := sha256.Sum256(payload.Bytes())
	root := t.TempDir()
	runner := NewRunner(NewStoreWithRunRoot(filepath.Join(root, "miniapps"), filepath.Join(root, "apps")), nil)
	app := greetingApp()
	app.ID = "portable-runtime-app"
	app.Requirements.Runtimes = []Runtime{{
		ID: "demo-runtime", Version: "1.0.0",
		Provision: &ProvisionPolicy{
			Mode: "portable", Interaction: "silent_private", URL: server.URL,
			SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(payload.Len()),
		},
	}}

	first, err := runner.prepareDependencies(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.prepareDependencies(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("download count = %d, want 1", hits)
	}
	firstPath := first["demo-runtime"].(string)
	if second["demo-runtime"] != firstPath {
		t.Fatalf("cached runtime changed: %v != %v", second["demo-runtime"], firstPath)
	}
	cache := filepath.Join(root, "apps", app.ID, "cache")
	relative, err := filepath.Rel(cache, firstPath)
	if err != nil || relative == ".." || filepath.IsAbs(relative) {
		t.Fatalf("runtime escaped private cache: %q (%v)", firstPath, err)
	}
	if _, err := os.Stat(filepath.Join(cache, "manifest.lock.json")); err != nil {
		t.Fatalf("missing dependency lock: %v", err)
	}
}

func TestPortableRuntimeRejectsChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-a-zip"))
	}))
	defer server.Close()
	runner := NewRunner(NewStoreWithRunRoot(filepath.Join(t.TempDir(), "miniapps"), filepath.Join(t.TempDir(), "apps")), nil)
	app := greetingApp()
	app.Requirements.Runtimes = []Runtime{{
		ID: "demo-runtime", Version: "1",
		Provision: &ProvisionPolicy{
			Mode: "portable", Interaction: "silent_private", URL: server.URL,
			SHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SizeBytes: 9,
		},
	}}
	if _, err := runner.prepareDependencies(context.Background(), app); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}
