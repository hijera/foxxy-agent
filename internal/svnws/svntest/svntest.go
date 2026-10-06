// Package svntest builds and drives a fake Subversion client so tests can
// exercise the real os/exec path, argument construction, and XML parsing on
// machines without svn installed. The fake is a small Go program (subpackage
// fakesvn) that keeps its repository model in a JSON state file and records
// every invocation.
package svntest

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

// Env var names the fake client reads. Tests set them with t.Setenv; the client
// inherits them because exec does not override the environment.
const (
	EnvState = "FOXXYCODE_FAKE_SVN_STATE"
	EnvLog   = "FOXXYCODE_FAKE_SVN_LOG"
)

// WC is one registered working copy in the fake repository model.
type WC struct {
	Branch   string `json:"branch"`
	Revision int    `json:"revision"`
}

// State is the fake repository model, persisted as JSON between invocations.
type State struct {
	RepositoryRoot string            `json:"repository_root"`
	UUID           string            `json:"uuid"`
	Branches       []string          `json:"branches"`
	WorkingCopies  map[string]*WC    `json:"working_copies"`
	Unversioned    []string          `json:"unversioned"`
	Status         string            `json:"status"`
	Diff           string            `json:"diff"`
	Log            string            `json:"log"`
	Fail           map[string]string `json:"fail"`
	// OutputCodePage encodes the fake client's plain-text output in a Windows
	// code page instead of UTF-8, reproducing the bytes a real svn.exe writes on
	// an install whose ANSI code page is not UTF-8. Zero means UTF-8; the XML
	// documents stay UTF-8 either way, as they do with the real client.
	OutputCodePage int `json:"output_code_page,omitempty"`
}

// Call is one recorded invocation of the fake client.
type Call struct {
	Dir  string   `json:"dir"`
	Args []string `json:"args"`
}

// Fake is a built fake svn client plus the paths backing its state and log.
type Fake struct {
	Binary string
	State  string
	Log    string
}

// Build compiles the fake client into dir and returns its handle. State and log
// files live next to the binary; call Setenv (or set EnvState/EnvLog yourself)
// before running anything that shells out to svn.
func Build(dir string) (Fake, error) {
	name := "svn"
	if runtime.GOOS == "windows" {
		name = "svn.exe"
	}
	f := Fake{
		Binary: filepath.Join(dir, name),
		State:  filepath.Join(dir, "state.json"),
		Log:    filepath.Join(dir, "calls.log"),
	}
	cmd := exec.Command("go", "build", "-o", f.Binary,
		"github.com/hijera/foxxycode-agent/internal/svnws/svntest/fakesvn")
	cmd.Dir = moduleDir()
	platform.HideConsoleWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Fake{}, fmt.Errorf("build fake svn: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return f, nil
}

// moduleDir returns the repository root, derived from this file's location.
func moduleDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// <root>/internal/svnws/svntest/svntest.go
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
}

// Setenv points the fake client at this handle's state and log files using the
// provided setter (typically t.Setenv).
func (f Fake) Setenv(set func(key, value string)) {
	set(EnvState, f.State)
	set(EnvLog, f.Log)
}

// WriteState persists the repository model and creates a .svn marker directory
// for every registered working copy, so working-copy detection that looks for
// the administrative directory behaves like the real client.
func (f Fake) WriteState(s State) error {
	for path := range s.WorkingCopies {
		if err := os.MkdirAll(filepath.Join(path, ".svn"), 0o755); err != nil {
			return fmt.Errorf("fake wc marker: %w", err)
		}
	}
	buf, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.State, buf, 0o644)
}

// ReadState loads the current repository model, reflecting mutations made by
// commits, updates, switches, and checkouts.
func (f Fake) ReadState() (State, error) {
	var s State
	raw, err := os.ReadFile(f.State)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, err
	}
	return s, nil
}

// Calls returns every recorded invocation in order.
func (f Fake) Calls() ([]Call, error) {
	raw, err := os.ReadFile(f.Log)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Call
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var c Call
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ResetCalls clears the invocation log.
func (f Fake) ResetCalls() error {
	err := os.Remove(f.Log)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// FindCall returns the first recorded invocation whose arguments contain sub as
// the subcommand (the first non-flag argument).
func (f Fake) FindCall(sub string) (Call, bool) {
	calls, err := f.Calls()
	if err != nil {
		return Call{}, false
	}
	for _, c := range calls {
		for _, a := range c.Args {
			if strings.HasPrefix(a, "-") {
				continue
			}
			if a == sub {
				return c, true
			}
			break
		}
	}
	return Call{}, false
}

// NewState returns a repository model with one working copy on trunk.
func NewState(repoRoot, wcPath string) State {
	return State{
		RepositoryRoot: repoRoot,
		UUID:           "11111111-2222-3333-4444-555555555555",
		Branches:       []string{"trunk", "branches/feature-x", "branches/release-1"},
		WorkingCopies:  map[string]*WC{wcPath: {Branch: "trunk", Revision: 12}},
		Status:         "M       src/main.go\n?       notes.txt",
		Diff:           "Index: src/main.go\n===================================================================\n--- src/main.go\t(revision 12)\n+++ src/main.go\t(working copy)\n@@ -1,3 +1,4 @@\n package main\n+// changed\n",
		Log:            "------------------------------------------------------------------------\nr12 | dev | 2026-07-01 10:00:00 +0300 | 1 line\n\ninitial import\n------------------------------------------------------------------------",
	}
}

// ANSICodePage reports the system ANSI code page, or 0 where there is none -
// every platform but Windows. This is the page the real client converts its
// output to, so a test that wants the fake to write what svn.exe writes points
// State.OutputCodePage at it.
func ANSICodePage() int {
	if _, cp, ok := platform.DecodeANSI([]byte{0xC0}); ok {
		return int(cp)
	}
	return 0
}

// NonASCIISample returns text the client on this machine can carry, and false
// when there is none to offer. svn replaces whatever the ANSI code page cannot
// hold - Cyrillic is lost on a Western install and umlauts on a Russian one - so
// the sample has to follow the machine rather than the other way round.
func NonASCIISample() (string, bool) {
	switch ANSICodePage() {
	case 0, 65001, 1251: // no legacy page, UTF-8, or the Cyrillic page
		return "Привет-Мир", true
	case 1252:
		return "Grüße-Straße", true
	}
	return "", false
}
