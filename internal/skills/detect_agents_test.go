package skills_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The crossreview detection script is the first-run entry point: it must name
// exactly the CLIs that are installed and verified, never the full candidate
// list.
func TestDetectAgentsScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detect-agents.sh is the POSIX half; Windows uses detect-agents.ps1")
	}
	bin := t.TempDir()
	stub := func(name string, rc int) {
		body := "#!/bin/sh\nexit " + string(rune('0'+rc)) + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stub("claude", 0) // no models command
	stub("codex", 0)  // every probe exits 0, so its models_cmd is reported
	stub("devin", 1)  // a name on PATH that is not the agent: --version fails
	// opencode, cursor-agent, koda deliberately absent.

	cmd := exec.Command("sh", "bundled/crossreview/detect-agents.sh")
	cmd.Env = []string{"PATH=" + bin}
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("detect-agents.sh: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	got := map[string][]string{}
	for _, ln := range lines {
		f := strings.Split(ln, "\t")
		if len(f) != 4 {
			t.Fatalf("line %q is not agent<TAB>path<TAB>models_cmd<TAB>template", ln)
		}
		got[f[0]] = f
	}
	if len(got) != 2 {
		t.Fatalf("detected agents = %v, want claude and codex only", lines)
	}
	if got["claude"][1] != filepath.Join(bin, "claude") || got["claude"][2] != "" {
		t.Fatalf("claude row = %v", got["claude"])
	}
	if got["codex"][2] != "codex debug models" {
		t.Fatalf("codex models_cmd = %q", got["codex"][2])
	}
	for _, row := range got {
		if !strings.Contains(row[3], "{brief}") || !strings.Contains(row[3], "{out}") {
			t.Fatalf("template keeps the placeholders: %q", row[3])
		}
	}
}
