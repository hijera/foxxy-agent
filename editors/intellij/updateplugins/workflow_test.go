// Tests for the release step that publishes updatePlugins.xml to GitHub Pages.
//
// That step only ever runs on a real release, so a mistake in it surfaces as a release whose
// repository document never moved — 0.2.89 shipped exactly that, because the script took the
// zip path relative to the workspace and then changed directory into the main checkout. These
// tests read the step script straight out of the workflow file and run it the way the runner
// does: from the workspace root, next to a build output, a checkout of main and a git remote.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	releaseWorkflow = "../../../.github/workflows/intellij-plugin.yaml"
	publishStep     = "Publish updatePlugins.xml to GitHub Pages"
)

func TestReleasePublishStepAdvertisesTheNewRelease(t *testing.T) {
	fixture := newPublishFixture(t, "1.2.2")

	if out, err := fixture.run("1.2.3"); err != nil {
		t.Fatalf("publish step failed: %v\n%s", err, out)
	}

	fixture.wantPublished("1.2.3")
	if subject := fixture.originGit("log", "-1", "--format=%s", "main"); subject != "chore(intellij): advertise 1.2.3 in the plugin repository" {
		t.Fatalf("latest commit on main = %q", subject)
	}
}

// The document is untracked on a main that never had one, where "git diff" sees nothing.
func TestReleasePublishStepCreatesTheFirstDocument(t *testing.T) {
	fixture := newPublishFixture(t, "")

	if out, err := fixture.run("1.2.3"); err != nil {
		t.Fatalf("publish step failed: %v\n%s", err, out)
	}

	fixture.wantPublished("1.2.3")
}

// Rebuilding an older tag must leave the newer document, and main, alone.
func TestReleasePublishStepKeepsANewerDocument(t *testing.T) {
	fixture := newPublishFixture(t, "1.3.0")
	before := fixture.originGit("rev-parse", "main")

	if out, err := fixture.run("1.2.3"); err != nil {
		t.Fatalf("publish step failed: %v\n%s", err, out)
	}

	fixture.wantPublished("1.3.0")
	if after := fixture.originGit("rev-parse", "main"); after != before {
		t.Fatalf("main moved from %s to %s for an older release", before, after)
	}
}

// Another pull request merging between the reset and the push must not lose either change.
func TestReleasePublishStepRetriesWhenMainMoves(t *testing.T) {
	fixture := newPublishFixture(t, "1.2.2")
	fixture.competeOnFirstPush()

	if out, err := fixture.run("1.2.3"); err != nil {
		t.Fatalf("publish step failed: %v\n%s", err, out)
	}

	fixture.wantPublished("1.2.3")
	if log := fixture.originGit("log", "--format=%s", "main"); !strings.Contains(log, "competing merge") {
		t.Fatalf("the competing commit is gone from main:\n%s", log)
	}
}

type publishFixture struct {
	t          *testing.T
	bash       string
	script     string
	env        []string
	workspace  string
	origin     string
	seed       string
	runnerTemp string
}

// newPublishFixture lays out what the runner has when the step starts: a bare remote whose main
// optionally already advertises seededVersion, the built zip under the workspace, main checked
// out into main-checkout, and the generator built into RUNNER_TEMP.
func newPublishFixture(t *testing.T, seededVersion string) *publishFixture {
	t.Helper()
	f := &publishFixture{t: t, bash: requireGitBash(t)}

	script := workflowStepScript(t, releaseWorkflow, publishStep)
	f.script = strings.ReplaceAll(script, "${{ github.repository }}", "hijera/foxxy-agent")
	if strings.Contains(f.script, "${{") {
		t.Fatalf("the step uses a workflow expression this test does not provide:\n%s", f.script)
	}

	// A clean git, like a fresh runner: no user config (signing, hooks, autocrlf) leaks in.
	emptyConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(emptyConfig, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+emptyConfig,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)

	f.origin = filepath.Join(t.TempDir(), "origin.git")
	f.git("", "init", "--quiet", "--bare", "--initial-branch=main", f.origin)

	f.seed = t.TempDir()
	f.git(f.seed, "init", "--quiet", "--initial-branch=main")
	f.write(filepath.Join(f.seed, "docs", "README.md"), "docs\n")
	if seededVersion != "" {
		document, err := render(descriptor{
			ID:         "dev.foxxycode.intellij",
			Name:       "FoxxyCode",
			Version:    seededVersion,
			Vendor:     "FoxxyCode",
			SinceBuild: "222",
		}, downloadURL("hijera/foxxy-agent", seededVersion, "foxxycode-intellij-"+seededVersion+".zip"))
		if err != nil {
			t.Fatal(err)
		}
		f.write(filepath.Join(f.seed, "docs", "updatePlugins.xml"), string(document))
	}
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "--quiet", "-m", "init")
	f.git(f.seed, "remote", "add", "origin", f.origin)
	f.git(f.seed, "push", "--quiet", "origin", "main")

	// The build output stays where make intellij-build leaves it, relative to the workspace.
	f.workspace = t.TempDir()
	built := pluginZip(t, "foxxycode-intellij-1.2.3.zip",
		descriptorXML("dev.foxxycode.intellij", "FoxxyCode", "1.2.3", "222", ""))
	distributions := filepath.Join(f.workspace, "editors", "intellij", "build", "distributions")
	if err := os.MkdirAll(distributions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(built, filepath.Join(distributions, filepath.Base(built))); err != nil {
		t.Fatal(err)
	}

	// actions/checkout with ref: main and path: main-checkout.
	f.git(f.workspace, "clone", "--quiet", "--branch", "main", f.origin, "main-checkout")

	f.runnerTemp = t.TempDir()
	generator := filepath.Join(f.runnerTemp, "updateplugins")
	if runtime.GOOS == "windows" {
		generator += ".exe"
	}
	build := exec.Command("go", "build", "-o", generator, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the generator: %v\n%s", err, out)
	}
	return f
}

// run executes the step script the way ubuntu-latest does: bash -e -o pipefail, from the
// workspace root, with the step's environment.
func (f *publishFixture) run(version string) (string, error) {
	cmd := exec.Command(f.bash, "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", f.script)
	cmd.Dir = f.workspace
	cmd.Env = append(f.env,
		"GITHUB_WORKSPACE="+f.workspace,
		"RUNNER_TEMP="+f.runnerTemp,
		"VERSION="+version,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (f *publishFixture) wantPublished(version string) {
	f.t.Helper()
	document := f.originGit("show", "main:docs/updatePlugins.xml")
	if !strings.Contains(document, `version="`+version+`"`) {
		f.t.Fatalf("main does not advertise %s:\n%s", version, document)
	}
}

// competeOnFirstPush makes the first push from the main checkout lose a race: a pre-push hook
// lands another commit on the remote once, then removes itself.
func (f *publishFixture) competeOnFirstPush() {
	f.t.Helper()
	hook := filepath.Join(f.workspace, "main-checkout", ".git", "hooks", "pre-push")
	f.write(hook, "#!/bin/sh\n"+
		"rm -f \"$0\"\n"+
		"cd '"+filepath.ToSlash(f.seed)+"' || exit 0\n"+
		"echo 'someone else merged' >> docs/README.md\n"+
		"git commit --quiet -am 'competing merge' && git push --quiet origin main\n")
	if err := os.Chmod(hook, 0o755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *publishFixture) originGit(args ...string) string {
	f.t.Helper()
	return strings.TrimSpace(f.git("", append([]string{"--git-dir", f.origin}, args...)...))
}

func (f *publishFixture) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (f *publishFixture) write(path, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// workflowStepScript returns the run: script of the named step in any job of a workflow file.
func workflowStepScript(t *testing.T, path, step string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, job := range workflow.Jobs {
		for _, candidate := range job.Steps {
			if candidate.Name == step {
				return candidate.Run
			}
		}
	}
	t.Fatalf("no step %q in %s", step, path)
	return ""
}

// requireGitBash finds a bash that can run the step against this machine's paths. The step runs
// on ubuntu-latest; on Windows only Git Bash qualifies — the bash.exe in System32 is WSL, which
// cannot see Windows paths, and it usually comes first on PATH.
func requireGitBash(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not available")
	}
	if runtime.GOOS != "windows" {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash is not available")
		}
		return bash
	}
	// git.exe lives at <Git>\cmd, <Git>\bin or <Git>\mingw64\bin depending on which PATH entry
	// won, so walk up from it to the installation root that holds bash.
	for dir := filepath.Dir(gitPath); ; dir = filepath.Dir(dir) {
		for _, candidate := range []string{
			filepath.Join(dir, "bin", "bash.exe"),
			filepath.Join(dir, "usr", "bin", "bash.exe"),
		} {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	t.Skip("Git Bash not found next to git; the release step runs on ubuntu-latest")
	return ""
}
