package cmdprofile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/platform"
)

// BinaryNotFoundError reports that the profile's binary could not be resolved.
// Callers append install guidance (the detected package-manager command) to it.
type BinaryNotFoundError struct {
	Binary string
	Err    error
}

func (e *BinaryNotFoundError) Error() string {
	return fmt.Sprintf("command %q is not installed or not on PATH", e.Binary)
}

func (e *BinaryNotFoundError) Unwrap() error { return e.Err }

// ExecOptions bounds one profile run.
type ExecOptions struct {
	// CWD is the working directory of the child process.
	CWD string
	// ForbiddenRoot, when set, is a directory tree the resolved binary must
	// NOT live in — the run workspace, so a prior step cannot write a binary
	// and have the command step execute it.
	ForbiddenRoot string
	// TimeoutSeconds overrides the profile timeout when positive.
	TimeoutSeconds int
}

// lookPath is a test seam over exec.LookPath.
var lookPath = exec.LookPath

// ResolveBinary resolves the profile's binary to the absolute path that would
// be executed, applying every safety rule: no implicit current-directory
// resolution (exec.ErrDot), absolute result only, no batch files, and not
// inside forbiddenRoot.
func ResolveBinary(spec ProfileSpec, forbiddenRoot string) (string, error) {
	binary := strings.TrimSpace(spec.Binary)
	if binary == "" {
		return "", &BinaryNotFoundError{Binary: spec.Name, Err: errors.New("profile declares no binary")}
	}
	resolved, err := lookPath(binary)
	if err != nil {
		// Go resolves a bare name found in the process working directory to a
		// relative ./name and flags it with ErrDot; executing it would run
		// whatever happens to sit in the CWD. Refuse explicitly.
		if errors.Is(err, exec.ErrDot) {
			return "", fmt.Errorf("binary %q resolves only via the current directory, which is not allowed", binary)
		}
		return "", &BinaryNotFoundError{Binary: binary, Err: err}
	}
	if !filepath.IsAbs(resolved) {
		abs, absErr := filepath.Abs(resolved)
		if absErr != nil {
			return "", fmt.Errorf("resolve %q to an absolute path: %w", resolved, absErr)
		}
		resolved = abs
	}
	lower := strings.ToLower(resolved)
	// CreateProcess launches batch files through cmd.exe, whose command-line
	// re-parsing is an injection vector; profiles may only run real executables.
	if strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd") {
		return "", fmt.Errorf("binary %q is a batch file, which command profiles refuse to execute", resolved)
	}
	if forbiddenRoot != "" {
		if inside, checkErr := pathInsideRoot(forbiddenRoot, resolved); checkErr == nil && inside {
			return "", fmt.Errorf("binary %q resolves inside the run workspace %q, which is not allowed", resolved, forbiddenRoot)
		}
	}
	return resolved, nil
}

// pathInsideRoot reports whether path lives under root, following symlinks on
// both sides best-effort so a link cannot dodge the check.
func pathInsideRoot(root, path string) (bool, error) {
	canonicalRoot := canonicalPath(root)
	canonicalTarget := canonicalPath(path)
	rel, err := filepath.Rel(canonicalRoot, canonicalTarget)
	if err != nil {
		// Different volumes on Windows: definitely outside.
		return false, nil
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

func canonicalPath(path string) string {
	cleaned := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}
	return cleaned
}

// Run builds the argv from the profile and params, resolves the binary, and
// executes it. The mirror of internal/svnws run: bounded by a timeout, hidden
// console window on Windows, ANSI-decoded output, stderr folded into the error.
func Run(ctx context.Context, spec ProfileSpec, params map[string]string, opts ExecOptions) (string, error) {
	argv, err := BuildArgv(spec, params)
	if err != nil {
		return "", err
	}
	resolved, err := ResolveBinary(spec, opts.ForbiddenRoot)
	if err != nil {
		return "", err
	}
	timeout := time.Duration(spec.ResolvedTimeout()) * time.Second
	if opts.TimeoutSeconds > 0 {
		timeout = time.Duration(opts.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, resolved, argv...)
	cmd.Dir = opts.CWD
	platform.HideConsoleWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	// Tools write in the system ANSI code page on legacy Windows installs;
	// decode so the caller never reads mojibake.
	stdoutText := platform.DecodeANSIOutput(stdout.Bytes())
	stderrText := platform.DecodeANSIOutput(stderr.Bytes())
	out := strings.TrimRight(stdoutText, "\r\n")
	if runErr != nil {
		detail := strings.TrimSpace(stderrText)
		if detail == "" {
			detail = strings.TrimSpace(stdoutText)
		}
		if ctx.Err() != nil {
			return out, fmt.Errorf("%s timed out after %s: %s", spec.ToolName(), timeout, detail)
		}
		return out, fmt.Errorf("%s: %w: %s", spec.ToolName(), runErr, detail)
	}
	return out, nil
}

// writeFile is a small helper shared by tests.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
