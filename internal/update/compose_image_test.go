package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestComposeImageMatchesDefaultRepo keeps `docker compose pull` pointed at the
// image the release workflow publishes. docker-build-push.yaml pushes to
// ghcr.io/${GITHUB_REPOSITORY}, the repository DefaultRepo names (pinned by
// TestRun_withoutRepoAsksTheRepositoryReleasesArePublishedTo), but the default
// in docker-compose.yml is a hardcoded string that cannot follow it. It read
// ghcr.io/hijera/foxxycode-agent - the spelling of the Go module path, which
// names no package - so a default pull had nothing to fetch.
func TestComposeImageMatchesDefaultRepo(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "docker-compose.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	got, ok := composeImageDefault(string(b))
	if !ok {
		t.Fatalf("no image: entry in %s", path)
	}
	if want := "ghcr.io/" + DefaultRepo + ":latest"; got != want {
		t.Fatalf("%s pulls %s by default, want %s: the release workflow publishes the image as ghcr.io/<owner>/<repo>", path, got, want)
	}
}

// composeImageDefault returns the first image: value in a compose file,
// unwrapping a ${VAR:-default} substitution to its default.
func composeImageDefault(compose string) (string, bool) {
	for _, line := range strings.Split(compose, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "image:")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		inner, isVar := strings.CutPrefix(value, "${")
		inner, closed := strings.CutSuffix(inner, "}")
		if _, def, hasDefault := strings.Cut(inner, ":-"); isVar && closed && hasDefault {
			return def, true
		}
		return value, true
	}
	return "", false
}
