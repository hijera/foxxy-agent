package platform

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// spawnSiteExemptions are the files that build an exec.Cmd without calling
// AdaptCommand, each with the reason it never starts a program on Android.
var spawnSiteExemptions = map[string]string{
	"internal/svnws/svntest/":     "test fixture helpers, never linked into a shipped binary",
	"internal/serve/systemd.go":   "systemctl, behind NewUserService, which refuses every GOOS but linux",
	"internal/update/packages.go": "dpkg-query, rpm and brew, asked on linux and darwin only",
	"internal/docsgen/":           "the documentation generator behind make docs, never shipped",
}

// TestEverySpawnSiteAdaptsTheCommand holds the rule AdaptCommand relies on. On
// Android a program FoxxyCode starts only runs once AdaptCommand has fitted the
// command to the device, and nothing but a device notices a spawn site that
// forgot it: every other host treats the call as a no-op. So every file outside
// the tests that builds an exec.Cmd calls it, or is exempted above with its
// reason.
func TestEverySpawnSiteAdaptsTheCommand(t *testing.T) {
	root := filepath.Join("..", "..")
	used := map[string]bool{}
	for _, top := range []string{"cmd", "internal", "external"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_windows.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := string(data)
			if first, _, _ := strings.Cut(src, "\n"); strings.TrimSpace(first) == "//go:build windows" {
				return nil
			}
			if !strings.Contains(src, "exec.Command(") && !strings.Contains(src, "exec.CommandContext(") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			for exempt := range spawnSiteExemptions {
				if rel == exempt || (strings.HasSuffix(exempt, "/") && strings.HasPrefix(rel, exempt)) {
					used[exempt] = true
					return nil
				}
			}
			if !strings.Contains(src, "AdaptCommand(") {
				t.Errorf("%s builds an exec.Cmd without platform.AdaptCommand: call it before Start, or exempt the file with the reason it never runs on Android", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for exempt, reason := range spawnSiteExemptions {
		if !used[exempt] {
			t.Errorf("exemption %s (%s) names no file that starts a program: drop it", exempt, reason)
		}
	}
}
