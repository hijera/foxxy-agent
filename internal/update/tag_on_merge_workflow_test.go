// Tests for the version "Tag release on merge" gives a merged pull request.
//
// The step bumps the patch of the highest SemVer tag on origin, so the minor version never moves
// by itself. MIN_VERSION is the floor of the current release line: a candidate below it is
// replaced by the floor, which is how the 0.2.x line turned into 0.3.0 without anyone pushing a
// tag by hand (a hand-pushed tag releases only the binaries and the image, not the plugins).
package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	tagBumpStep = "Get latest SemVer tag and bump patch"

	// releaseLineFloor is the floor the workflow is expected to carry. Moving to the next minor
	// line is a one-line change there, and this constant moves with it.
	releaseLineFloor = "0.3.0"
)

func TestTagOnMergeDeclaresTheReleaseLineFloor(t *testing.T) {
	t.Parallel()
	env := workflowStepEnv(t, tagOnMergeWorkflow, tagBumpStep)
	if got := env["MIN_VERSION"]; got != releaseLineFloor {
		t.Fatalf("MIN_VERSION = %q, want %q (env of the step: %v)", got, releaseLineFloor, env)
	}
}

func TestTagOnMergePicksTheNextVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"the last 0.2.x release is followed by the floor", []string{"0.2.94", "0.2.95"}, "0.3.0"},
		{"the floor itself is followed by a patch bump", []string{"0.2.95", "0.3.0"}, "0.3.1"},
		{"patches compare as numbers", []string{"0.3.9", "0.3.10"}, "0.3.11"},
		{"no tags at all start at the floor", nil, "0.3.0"},
		{"tags that are not X.Y.Z are ignored", []string{"0.2.95", "v1.0.0", "1.1.32-rc", "screenshots"}, "0.3.0"},
		{"a line above the floor is not pulled back", []string{"0.3.7", "0.4.2"}, "0.4.3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := runTagBump(t, tc.tags); got != tc.want {
				t.Fatalf("tags %v released as %q, want %q", tc.tags, got, tc.want)
			}
		})
	}
}

// runTagBump runs the step in a repository carrying the given tags and returns the version it
// wrote to GITHUB_OUTPUT.
func runTagBump(t *testing.T, tags []string) string {
	t.Helper()
	bash := requireGitBash(t)
	script := workflowStepRun(t, tagOnMergeWorkflow, tagBumpStep)
	if strings.Contains(script, "${{") {
		t.Fatalf("the step uses a workflow expression in its script; pass it in through env:\n%s", script)
	}

	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.test",
			"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "--quiet")
	git("commit", "--quiet", "--allow-empty", "-m", "root")
	for _, tag := range tags {
		git("tag", tag)
	}

	output := filepath.Join(t.TempDir(), "github_output")
	if err := os.WriteFile(output, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GITHUB_OUTPUT="+filepath.ToSlash(output))
	for key, value := range workflowStepEnv(t, tagOnMergeWorkflow, tagBumpStep) {
		env = append(env, key+"="+value)
	}

	cmd := exec.Command(bash, "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", script)
	cmd.Dir = repo
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the step failed: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			outputs[key] = value
		}
	}
	if outputs["version"] != outputs["tag"] {
		t.Fatalf("version %q and tag %q differ; the release jobs are called with the tag", outputs["version"], outputs["tag"])
	}
	return outputs["version"]
}

// workflowStepEnv returns the env: block of the named step in any job of a workflow.
func workflowStepEnv(t *testing.T, workflow, step string) map[string]string {
	t.Helper()
	for _, job := range readWorkflow(t, workflow).Jobs {
		for _, candidate := range job.Steps {
			if candidate.Name == step {
				return candidate.Env
			}
		}
	}
	t.Fatalf("no step %q in %s", step, workflow)
	return nil
}
