package vscode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/platform"
)

// The extension's CI job installs its Node dependencies before this suite.
// Go-only builds can still check the core without installing an editor plugin.
func TestBackendRestartFeature(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("install Node and run npm ci in editors/vscode to check the extension")
	}
	if _, err := os.Stat("node_modules/vitest/vitest.mjs"); os.IsNotExist(err) {
		t.Skip("run npm ci in editors/vscode to check the extension")
	}
	run := func(file, name string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "node", "node_modules/vitest/vitest.mjs", "run",
			"--maxWorkers=1", "--minWorkers=1", file, "--testNamePattern", regexp.QuoteMeta(name)+"$")
		platform.AdaptCommand(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %s: %w\n%s", file, name, err, out)
		}
		// Vitest exits zero when a mistyped test name selects no tests.
		plain := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAll(out, nil)
		if !regexp.MustCompile(`(?m)^\s*Tests\s+[1-9]\d* passed\b`).Match(plain) {
			return fmt.Errorf("no passing test named %q\n%s", name, out)
		}
		return nil
	}
	suite := godog.TestSuite{
		Name: "vscode_backend_restart",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^the replacement remains running when the previous backend exits$`, func() error {
				return run("test/processManager.test.ts", "keeps the replacement running when the previous backend exits")
			})
			sc.Step(`^the editor tab and sidebar reconnect to the replacement URL$`, func() error {
				return run("test/extensionRestart.test.ts", "updates every open panel to the replacement backend URL")
			})
			sc.Step(`^the remaining sidebar reconnects and the editor tab stays closed$`, func() error {
				return run("test/extensionRestart.test.ts", "updates the remaining sidebar when the editor tab closes during restart")
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/vscode_backend_restart.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("VS Code backend restart feature failed")
	}
}
