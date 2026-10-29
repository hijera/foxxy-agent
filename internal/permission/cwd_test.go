package permission

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunCommandCWDOutside(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	tests := []struct {
		name string
		args string
		want bool
	}{
		{"no cwd runs in the workspace", `{"command":"go test"}`, false},
		{"empty cwd is the workspace", `{"command":"go test","cwd":""}`, false},
		{"relative inside", `{"command":"go test","cwd":"pkg/x"}`, false},
		{"the workspace itself", `{"command":"go test","cwd":"."}`, false},
		{"relative climbing out", `{"command":"go test","cwd":"../.."}`, true},
		{"absolute elsewhere", `{"command":"go test","cwd":"` + filepath.ToSlash(outside) + `"}`, true},
		{"a sibling sharing a prefix", `{"command":"go test","cwd":"` + filepath.ToSlash(ws) + `-other"}`, true},
		{"no workspace known", `{"command":"go test","cwd":"x"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := ws
			if tt.name == "no workspace known" {
				ws = ""
			}
			if got := RunCommandCWDOutside(ws, tt.args); got != tt.want {
				t.Fatalf("RunCommandCWDOutside(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestRunCommandCWDOutsideResolvesLinks(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(ws, "linked")
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
			t.Skipf("junction unavailable: %v: %s", err, out)
		}
	} else if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if !RunCommandCWDOutside(ws, `{"command":"go test","cwd":"linked"}`) {
		t.Fatalf("cwd %q resolves outside the workspace but was treated as inside", link)
	}
}
