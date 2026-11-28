package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerRuntimeDefaults(t *testing.T) {
	t.Parallel()
	file, err := os.ReadFile(filepath.Join(dockerRepoRoot, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	_, runtime, found := strings.Cut(string(file), "\nFROM scratch")
	if !found {
		t.Fatal("Dockerfile is missing the scratch runtime stage")
	}
	var user, healthcheck string
	for _, line := range strings.Split(runtime, "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "USER "); ok {
			user = value
		}
		if value, ok := strings.CutPrefix(line, "HEALTHCHECK "); ok {
			healthcheck = value
		}
	}
	if user != "10001:10001" {
		t.Errorf("runtime user = %q, want non-root 10001:10001", user)
	}
	if !strings.HasSuffix(healthcheck, `CMD ["/bin/foxxycode", "--version"]`) {
		t.Errorf("runtime healthcheck = %q, want an exec-form FoxxyCode version probe", healthcheck)
	}
}
