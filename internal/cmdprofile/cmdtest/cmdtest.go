// Package cmdtest builds the fake recording binary command-profile tests run
// instead of a real tool, mirroring internal/svnws/svntest. Injection is
// purely through the profile's Binary field — no PATH manipulation.
package cmdtest

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/platform"
)

// Environment variables the fake binary reads. They are inherited by the
// child process, so tests set them with t.Setenv.
const (
	EnvLog    = "FOXXYCODE_FAKE_CMD_LOG"
	EnvExit   = "FOXXYCODE_FAKE_CMD_EXIT"
	EnvStdout = "FOXXYCODE_FAKE_CMD_STDOUT"
	EnvStderr = "FOXXYCODE_FAKE_CMD_STDERR"
	EnvSleep  = "FOXXYCODE_FAKE_CMD_SLEEP_MS"
)

// Call is one recorded invocation of the fake binary.
type Call struct {
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
}

// Fake is a built fake binary plus its call log.
type Fake struct {
	Binary string
	Log    string
}

// Build compiles the fake binary into dir under the given base name (e.g.
// "ffmpeg"); the .exe suffix is added on Windows.
func Build(dir, name string) (Fake, error) {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fake := Fake{
		Binary: filepath.Join(dir, name),
		Log:    filepath.Join(dir, "calls.log"),
	}
	cmd := exec.Command("go", "build", "-o", fake.Binary,
		"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest/fakecmd")
	cmd.Dir = moduleDir()
	platform.HideConsoleWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Fake{}, fmt.Errorf("build fake command: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return fake, nil
}

// moduleDir returns the repository root, derived from this file's location.
func moduleDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// <root>/internal/cmdprofile/cmdtest/cmdtest.go
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
}

// Setenv points the fake binary at this handle's call log using the provided
// setter (typically t.Setenv).
func (f Fake) Setenv(set func(key, value string)) {
	set(EnvLog, f.Log)
}

// Calls returns every recorded invocation, oldest first.
func (f Fake) Calls() ([]Call, error) {
	raw, err := os.ReadFile(f.Log)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var calls []Call
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var call Call
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			return nil, fmt.Errorf("decode fake call %q: %w", line, err)
		}
		calls = append(calls, call)
	}
	return calls, nil
}
