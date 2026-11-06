package shell

import (
	"context"
	"encoding/json"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	"os"
	"strings"
	"testing"
)

func TestRunCommandCWDOverrideOnlyAffectsOneCall(t *testing.T) {
	base := t.TempDir()
	other := t.TempDir()
	commandShell := platform.CurrentShell()
	command := "pwd"
	switch commandShell.Kind {
	case platform.ShellCmd:
		command = "cd"
	case platform.ShellPwsh, platform.ShellPowerShell:
		command = "(Get-Location).Path"
	}
	env := &tooling.Env{CWD: base}
	args, _ := json.Marshal(runCommandArgs{Command: command, CWD: other})
	out, err := executeRunCommandWithShell(context.Background(), string(args), env, commandShell)
	if err != nil {
		t.Fatal(err)
	}
	gotInfo, err := os.Stat(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("stat command cwd %q: %v", out, err)
	}
	wantInfo, err := os.Stat(other)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("override command cwd = %q, want %s", out, other)
	}
	if env.CWD != base {
		t.Fatalf("session cwd moved to %q", env.CWD)
	}
}
