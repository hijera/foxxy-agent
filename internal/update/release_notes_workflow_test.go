// Tests for how a GitHub Release gets its description.
//
// reportChanges prints the notes of every release an update skipped over, so a release published
// without a description is a hole in that report, and on GitHub it reads as an empty release.
// Those descriptions are written by release-notes.yaml, which "Tag release on merge" calls after
// every job that attaches assets to the release. One late job, because the binaries, IntelliJ and
// VS Code jobs attach to the same release at the same moment: softprops/action-gh-release starts
// each of them from a draft of its own and keeps the earliest, deleting the rest, so notes
// generated inside one of those jobs survived only when that job's draft won. 0.2.87 and 0.2.91
// lost that race, and 0.2.70-0.2.79 never got notes because the job that generated them failed.
package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	releaseNotesWorkflow = "release-notes.yaml"
	releaseNotesStep     = "Write the release notes"
	tagOnMergeWorkflow   = "tag-on-merge.yaml"

	// generatedNotes is what the fake generate-notes endpoint answers.
	generatedNotes = "## What's Changed\n* fix: something by @someone in https://github.com/example/release-notes-test/pull/1"
)

var workflowsDir = filepath.Join("..", "..", ".github", "workflows")

func TestReleaseNotesStepFillsAnEmptyRelease(t *testing.T) {
	t.Parallel()
	f := newReleaseNotesFixture(t, "1.2.3")
	f.setBody("")

	if out, err := f.run(); err != nil {
		t.Fatalf("release notes step failed: %v\n%s", err, out)
	}

	if body := strings.TrimSpace(f.body()); body != generatedNotes {
		t.Fatalf("release description = %q, want the generated notes %q", body, generatedNotes)
	}
	if !f.called("api", "repos/example/release-notes-test/releases/generate-notes", "tag_name=1.2.3") {
		t.Fatalf("notes were not generated for 1.2.3; gh calls:\n%s", strings.Join(f.calls(), "\n"))
	}
}

// A rebuilt release, or one whose notes someone edited by hand, keeps what it has.
func TestReleaseNotesStepKeepsExistingNotes(t *testing.T) {
	t.Parallel()
	f := newReleaseNotesFixture(t, "1.2.3")
	f.setBody("Written by hand.\n")

	if out, err := f.run(); err != nil {
		t.Fatalf("release notes step failed: %v\n%s", err, out)
	}

	if body := f.body(); body != "Written by hand.\n" {
		t.Fatalf("release description = %q, want it left alone", body)
	}
	if f.called("api") || f.called("release", "edit") {
		t.Fatalf("the step rewrote a release that already had notes; gh calls:\n%s", strings.Join(f.calls(), "\n"))
	}
}

// Nothing to write into is a failure, not a quiet success: by hand it is a mistyped tag, and in
// the pipeline it means every job that attaches to the release failed.
func TestReleaseNotesStepFailsWithoutARelease(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		tag  string
		// released publishes the fake release; lookedUp says the step must have asked for it.
		released, lookedUp bool
	}{
		{name: "no release for the tag", tag: "1.2.3", lookedUp: true},
		{name: "not a SemVer tag", tag: "latest", released: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newReleaseNotesFixture(t, tc.tag)
			if tc.released {
				f.setBody("")
			}

			if out, err := f.run(); err == nil {
				t.Fatalf("release notes step succeeded:\n%s", out)
			}

			calls := f.calls()
			if tc.lookedUp && !f.called("release", "view", tc.tag) {
				t.Fatalf("the step failed without asking for release %s; gh calls:\n%s", tc.tag, strings.Join(calls, "\n"))
			}
			if !tc.lookedUp && len(calls) > 0 {
				t.Fatalf("the step called gh for tag %q; gh calls:\n%s", tc.tag, strings.Join(calls, "\n"))
			}
			if f.called("release", "edit") {
				t.Fatalf("the step edited a release; gh calls:\n%s", strings.Join(calls, "\n"))
			}
		})
	}
}

func TestReleaseNotesRunAfterEveryJobThatAttachesToTheRelease(t *testing.T) {
	t.Parallel()
	pipeline := readWorkflow(t, tagOnMergeWorkflow)

	notesJob := ""
	for id, job := range pipeline.Jobs {
		if job.Uses == "./.github/workflows/"+releaseNotesWorkflow {
			notesJob = id
		}
	}
	if notesJob == "" {
		t.Fatalf("%s never calls %s, so nothing writes the notes of a release", tagOnMergeWorkflow, releaseNotesWorkflow)
	}
	notes := pipeline.Jobs[notesJob]
	if !strings.Contains(notes.If, "!cancelled()") && !strings.Contains(notes.If, "always()") {
		t.Errorf("job %s runs only when every job it needs has passed (if: %q), so a release with a failed build gets no notes", notesJob, notes.If)
	}

	attaching := 0
	for id, job := range pipeline.Jobs {
		child, ok := strings.CutPrefix(job.Uses, "./.github/workflows/")
		if id == notesJob || !ok || !attachesToRelease(readWorkflow(t, child)) {
			continue
		}
		attaching++
		if !slices.Contains(notes.Needs, id) {
			t.Errorf("job %s (%s) attaches to the release, but %s does not wait for it: the notes can be written before that job's draft becomes the release", id, child, notesJob)
		}
	}
	if attaching == 0 {
		t.Fatalf("no job in %s attaches to a release; this test no longer recognises how releases are published", tagOnMergeWorkflow)
	}
}

func TestNoReleaseAttachStepGeneratesNotes(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob(filepath.Join(workflowsDir, "*.y*ml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no workflows under %s (%v)", workflowsDir, err)
	}
	for _, path := range paths {
		name := filepath.Base(path)
		for id, job := range readWorkflow(t, name).Jobs {
			for _, step := range job.Steps {
				if !strings.HasPrefix(step.Uses, "softprops/action-gh-release") {
					continue
				}
				if value, ok := step.With["generate_release_notes"]; ok && fmt.Sprint(value) == "true" {
					t.Errorf("%s: job %s, step %q generates release notes. The jobs of a release attach to it at the same time and it keeps whichever draft came first, so these notes vanish whenever another job wins (0.2.87, 0.2.91). Leave them to %s.", name, id, step.Name, releaseNotesWorkflow)
				}
			}
		}
	}
}

// workflowFile is the part of a GitHub Actions workflow these tests read.
type workflowFile struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Needs jobNeeds       `yaml:"needs"`
	If    string         `yaml:"if"`
	Uses  string         `yaml:"uses"`
	Steps []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

// jobNeeds is a job's needs:, which a workflow may write as a single job id or as a list.
type jobNeeds []string

func (n *jobNeeds) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*n = jobNeeds{value.Value}
		return nil
	}
	var ids []string
	if err := value.Decode(&ids); err != nil {
		return err
	}
	*n = ids
	return nil
}

func readWorkflow(t *testing.T, name string) workflowFile {
	t.Helper()
	path := filepath.Join(workflowsDir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return workflow
}

// attachesToRelease reports whether a workflow creates a GitHub Release or uploads to one.
func attachesToRelease(workflow workflowFile) bool {
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "softprops/action-gh-release") ||
				strings.Contains(step.Run, "gh release upload") ||
				strings.Contains(step.Run, "gh release create") {
				return true
			}
		}
	}
	return false
}

// fakeGH stands in for the GitHub CLI: one release whose description lives in a file, and a log
// of every call. It answers the three calls the step is meant to make and fails on anything else,
// so a new call shows up here before it reaches a real release. Plain bash 3.2: macOS runs it.
const fakeGH = `#!/usr/bin/env bash
set -eu
state="$FAKE_GH_STATE"
printf '%s\n' "$*" >> "$state/calls"
unexpected() {
	echo "unexpected gh call: $*" >&2
	exit 1
}
case "$1 ${2:-}" in
"release view")
	if [ ! -f "$state/body" ]; then
		echo "release not found" >&2
		exit 1
	fi
	cat "$state/body"
	;;
"release edit")
	notes=""
	while [ $# -gt 0 ]; do
		if [ "$1" = "--notes-file" ]; then
			notes="$2"
		fi
		shift
	done
	if [ -z "$notes" ]; then
		echo "release edit without --notes-file" >&2
		exit 1
	fi
	cp "$notes" "$state/body"
	;;
"api "*)
	for arg in "$@"; do
		case "$arg" in
		*/releases/generate-notes)
			printf '%s\n' "$FAKE_GH_NOTES"
			exit 0
			;;
		esac
	done
	unexpected "$@"
	;;
*)
	unexpected "$@"
	;;
esac
`

type releaseNotesFixture struct {
	t      *testing.T
	bash   string
	script string
	env    []string
	dir    string
	state  string
}

// newReleaseNotesFixture lays out what the runner has when the step starts, with a fake gh first
// on PATH. The step reads its inputs from env, as the workflow passes them.
func newReleaseNotesFixture(t *testing.T, tag string) *releaseNotesFixture {
	t.Helper()
	f := &releaseNotesFixture{t: t, bash: requireGitBash(t), dir: t.TempDir(), state: t.TempDir()}

	f.script = workflowStepRun(t, releaseNotesWorkflow, releaseNotesStep)
	if strings.Contains(f.script, "${{") {
		t.Fatalf("the step uses a workflow expression in its script; pass it in through env:\n%s", f.script)
	}

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fakeGH), 0o755); err != nil {
		t.Fatal(err)
	}
	f.env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH_STATE="+f.state,
		"FAKE_GH_NOTES="+generatedNotes,
		// Should the real gh ever run instead of the fake, it has no credentials and no real
		// repository to write to.
		"GH_TOKEN=not-a-token",
		"GITHUB_TOKEN=",
		"GH_CONFIG_DIR="+t.TempDir(),
		"REPO=example/release-notes-test",
		"TAG="+tag,
		"RUNNER_TEMP="+t.TempDir(),
	)
	return f
}

// run executes the step the way ubuntu-latest does: bash -e -o pipefail with the step's env.
func (f *releaseNotesFixture) run() (string, error) {
	cmd := exec.Command(f.bash, "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", f.script)
	cmd.Dir = f.dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// setBody publishes the fake release with the given description.
func (f *releaseNotesFixture) setBody(body string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.state, "body"), []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *releaseNotesFixture) body() string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.state, "body"))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *releaseNotesFixture) calls() []string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.state, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// called reports whether a single gh call carried every one of the given arguments.
func (f *releaseNotesFixture) called(args ...string) bool {
	for _, call := range f.calls() {
		fields := strings.Fields(call)
		matched := true
		for _, arg := range args {
			if !slices.Contains(fields, arg) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// workflowStepRun returns the run: script of the named step in any job of a workflow.
func workflowStepRun(t *testing.T, workflow, step string) string {
	t.Helper()
	for _, job := range readWorkflow(t, workflow).Jobs {
		for _, candidate := range job.Steps {
			if candidate.Name == step {
				return candidate.Run
			}
		}
	}
	t.Fatalf("no step %q in %s", step, workflow)
	return ""
}

// requireGitBash finds a bash that can run the step against this machine's paths. The step runs
// on ubuntu-latest; on Windows only Git Bash qualifies, because the bash.exe in System32 is WSL,
// which cannot see Windows paths, and it usually comes first on PATH. Same search as
// editors/intellij/updateplugins/workflow_test.go.
func requireGitBash(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash is not available")
		}
		return bash
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not available, so neither is Git Bash")
	}
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
