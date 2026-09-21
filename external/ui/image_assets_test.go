//go:build ui

package ui

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The SPA's brand assets are symlinks into docs/assets, so the file the bundler
// opens lives outside the directory the image build copies into the Node stage.
// Docker carries a symlink over as a symlink, which leaves the link dangling
// unless the Dockerfile also stages its target at the same path - and a dangling
// import is not a build warning, it fails `npm run build:go` and the published
// image silently keeps whatever SPA it had. That is exactly how the sign-in
// wordmark stopped the image from building while the release binaries went out
// as usual, so the coupling gets a test rather than a note.
//
// The links are read out of the index rather than off the disk: a Windows
// checkout without symlink support materialises each one as an ordinary file
// holding its target, and the image is built from what git has either way.
func TestDockerfileStagesEverySymlinkedAsset(t *testing.T) {
	copied := dockerfileAssetPatterns(t)

	seen := 0
	for name, target := range symlinkedAssets(t) {
		seen++
		// The link is resolved against src/assets, and the Dockerfile stages the
		// repository's docs/assets at /docs/assets - the very path four levels up
		// from src/assets inside the image.
		if want := "../../../../docs/assets"; path.Dir(target) != want {
			t.Errorf("src/assets/%s points at %s; the image only stages %s/",
				name, target, want)
			continue
		}
		if !matchesAny(copied, path.Base(target)) {
			t.Errorf("src/assets/%s resolves to docs/assets/%s, which no COPY in the Dockerfile stages: the image build fails on the import",
				name, path.Base(target))
		}
	}

	if seen == 0 {
		t.Fatal("no symlinked assets found; this test no longer guards anything")
	}
}

// symlinkedAssets maps each symlink under src/assets to the path it points at,
// as the index records them (mode 120000, the blob holding the target).
func symlinkedAssets(t *testing.T) map[string]string {
	t.Helper()

	out, err := exec.Command("git", "ls-files", "-s", "--", filepath.Join("src", "assets")).Output()
	if err != nil {
		t.Fatalf("git ls-files src/assets: %v", err)
	}

	links := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		meta, file, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 2 || fields[0] != "120000" {
			continue
		}
		blob, err := exec.Command("git", "cat-file", "blob", fields[1]).Output()
		if err != nil {
			t.Fatalf("git cat-file %s (%s): %v", fields[1], file, err)
		}
		links[path.Base(filepath.ToSlash(file))] = strings.TrimSpace(string(blob))
	}
	return links
}

// Base names, possibly globbed, that the Dockerfile copies out of docs/assets.
func dockerfileAssetPatterns(t *testing.T) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}

	source := regexp.MustCompile(`(?m)^COPY\s+(.*)\s+/docs/assets/\s*$`)
	found := source.FindAllStringSubmatch(string(raw), -1)
	if found == nil {
		t.Fatal("no COPY into /docs/assets/ in the Dockerfile; the symlinked assets have nowhere to land")
	}

	var patterns []string
	for _, line := range found {
		for _, field := range strings.Fields(line[1]) {
			patterns = append(patterns, path.Base(filepath.ToSlash(field)))
		}
	}
	return patterns
}

func matchesAny(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if ok, err := path.Match(pattern, name); err == nil && ok {
			return true
		}
	}
	return false
}
