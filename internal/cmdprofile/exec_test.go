package cmdprofile

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest"
)

// buildFake compiles the recording binary once per test into its own dir and
// returns a profile pointed at it by absolute path.
func buildFake(t *testing.T, name string) (cmdtest.Fake, ProfileSpec) {
	t.Helper()
	fake, err := cmdtest.Build(t.TempDir(), name)
	if err != nil {
		t.Fatal(err)
	}
	fake.Setenv(t.Setenv)
	spec := ffmpegSpec()
	spec.Binary = fake.Binary
	return fake, spec
}

func TestRunExecutesTheExactArgv(t *testing.T) {
	fake, spec := buildFake(t, "ffmpeg")
	t.Setenv(cmdtest.EnvStdout, "audio extracted")
	workspace := t.TempDir()

	out, err := Run(context.Background(), spec, map[string]string{
		"input_path": "in with space.mp4", "codec": "libmp3lame", "output_path": "out.mp3",
	}, ExecOptions{CWD: workspace, ForbiddenRoot: workspace})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out != "audio extracted" {
		t.Fatalf("Run() output = %q", out)
	}
	calls, err := fake.Calls()
	if err != nil || len(calls) != 1 {
		t.Fatalf("calls = %#v, err %v", calls, err)
	}
	wantArgs := []string{"-i", "in with space.mp4", "-vn", "-acodec", "libmp3lame", "out.mp3"}
	if strings.Join(calls[0].Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("recorded argv = %#v, want %#v", calls[0].Args, wantArgs)
	}
	if filepath.Clean(calls[0].Dir) != filepath.Clean(workspace) {
		t.Fatalf("recorded dir = %q, want %q", calls[0].Dir, workspace)
	}
}

func TestRunSurfacesFailureOutput(t *testing.T) {
	_, spec := buildFake(t, "ffmpeg")
	t.Setenv(cmdtest.EnvExit, "1")
	t.Setenv(cmdtest.EnvStderr, "Unknown encoder 'x'")
	workspace := t.TempDir()

	_, err := Run(context.Background(), spec, map[string]string{
		"input_path": "in.mp4", "codec": "aac", "output_path": "out.m4a",
	}, ExecOptions{CWD: workspace, ForbiddenRoot: workspace})
	if err == nil || !strings.Contains(err.Error(), "Unknown encoder") {
		t.Fatalf("Run() error = %v, want the stderr detail", err)
	}
}

func TestRunHonorsTheTimeout(t *testing.T) {
	_, spec := buildFake(t, "ffmpeg")
	t.Setenv(cmdtest.EnvSleep, "5000")
	workspace := t.TempDir()

	start := time.Now()
	_, err := Run(context.Background(), spec, map[string]string{
		"input_path": "in.mp4", "codec": "aac", "output_path": "out.m4a",
	}, ExecOptions{CWD: workspace, ForbiddenRoot: workspace, TimeoutSeconds: 1})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Run() error = %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("timeout did not fire promptly (%s)", elapsed)
	}
}

func TestResolveBinaryRejectsAMissingBinary(t *testing.T) {
	spec := ffmpegSpec()
	spec.Binary = filepath.Join(t.TempDir(), "does-not-exist")
	_, err := ResolveBinary(spec, "")
	var missing *BinaryNotFoundError
	if !errors.As(err, &missing) {
		t.Fatalf("ResolveBinary() error = %v, want BinaryNotFoundError", err)
	}
}

// A binary inside the run workspace is the classic escalation: a prior step
// writes a file, the command step "resolves" it. The resolver must refuse.
func TestResolveBinaryRejectsABinaryInsideTheForbiddenRoot(t *testing.T) {
	workspace := t.TempDir()
	fake, err := cmdtest.Build(workspace, "ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	spec := ffmpegSpec()
	spec.Binary = fake.Binary
	if _, err := ResolveBinary(spec, workspace); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("ResolveBinary() error = %v, want a workspace rejection", err)
	}
	// The same binary is fine when the forbidden root is elsewhere.
	if _, err := ResolveBinary(spec, t.TempDir()); err != nil {
		t.Fatalf("ResolveBinary() outside the root failed: %v", err)
	}
}

func TestResolveBinaryRejectsBatchFiles(t *testing.T) {
	dir := t.TempDir()
	batch := filepath.Join(dir, "convert.bat")
	if err := writeFile(batch, "@echo off\n"); err != nil {
		t.Fatal(err)
	}
	spec := ffmpegSpec()
	spec.Binary = batch
	if _, err := ResolveBinary(spec, ""); err == nil || !strings.Contains(err.Error(), "batch") {
		t.Fatalf("ResolveBinary() error = %v, want a batch-file rejection", err)
	}
}

// Go's LookPath resolves a bare name found in the current directory to a
// relative ./name and flags it with ErrDot; the resolver must not execute it.
func TestResolveBinaryRejectsErrDotResolution(t *testing.T) {
	dir := t.TempDir()
	fake, err := cmdtest.Build(dir, "onlyhere")
	if err != nil {
		t.Fatal(err)
	}
	_ = fake
	t.Chdir(dir)
	spec := ffmpegSpec()
	spec.Binary = "onlyhere"
	// PATH does not contain dir, so only the CWD fallback could find it.
	_, resolveErr := ResolveBinary(spec, "")
	var missing *BinaryNotFoundError
	if resolveErr == nil {
		t.Fatal("a CWD-relative resolution was accepted")
	}
	if !errors.As(resolveErr, &missing) && !strings.Contains(resolveErr.Error(), "current directory") {
		t.Fatalf("ResolveBinary() error = %v", resolveErr)
	}
}
