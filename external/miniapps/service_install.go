//go:build miniapps

package miniapps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/platform"
)

// JobCommandInstall is the async job kind for installing a command profile's
// binary through a package manager.
const JobCommandInstall JobKind = "command_install"

// commandInstallTimeout bounds one package-manager run; downloads can be slow.
const commandInstallTimeout = 10 * time.Minute

// installOutputTailLines caps how much manager output the job events carry.
const installOutputTailLines = 40

// StartCommandInstall launches an async job that runs the given package
// manager for the profile. The manager must come from DetectManagers over the
// same profile — its argv is entirely literal, built from the validated
// install coordinate, never from request input.
func (s *Service) StartCommandInstall(profile cmdprofile.ProfileSpec, manager cmdprofile.Manager) (AsyncJob, error) {
	if s == nil || s.store == nil {
		return AsyncJob{}, errors.New("mini app service is unavailable")
	}
	if len(manager.Argv) == 0 {
		return AsyncJob{}, errors.New("the package manager command is empty")
	}
	job, ctx := s.newJob(JobCommandInstall, "", "", "")
	s.updateJob(job.ID, func(item *AsyncJob) {
		item.Phase = "installing"
		item.Summary = strings.Join(manager.Argv, " ")
	})
	s.launchWorker(job.ID, func() { s.runCommandInstall(ctx, job.ID, profile, manager) })
	return s.GetJob(job.ID)
}

func (s *Service) runCommandInstall(ctx context.Context, id string, profile cmdprofile.ProfileSpec, manager cmdprofile.Manager) {
	s.updateJob(id, func(job *AsyncJob) {
		job.Status, job.Phase, job.Progress = JobRunning, "installing", 20
	})
	ctx, cancel := context.WithTimeout(ctx, commandInstallTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, manager.Argv[0], manager.Argv[1:]...)
	platform.HideConsoleWindow(cmd)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	runErr := cmd.Run()
	tail := outputTail(platform.DecodeANSIOutput(output.Bytes()), installOutputTailLines)

	if runErr != nil {
		detail := strings.TrimSpace(tail)
		if ctx.Err() != nil {
			detail = fmt.Sprintf("timed out after %s: %s", commandInstallTimeout, detail)
		}
		s.failJob(id, fmt.Errorf("%s failed: %s", manager.ID, detail))
		return
	}
	result := map[string]any{"manager": manager.ID, "package": manager.Package, "output_tail": tail}
	// Re-resolve so the caller learns immediately whether the binary is now
	// reachable; a manager that installed elsewhere still succeeds, with the
	// row simply staying "missing" until PATH or the config catches up.
	if resolved, err := cmdprofile.ResolveBinary(profile, ""); err == nil {
		result["binary"] = resolved
	}
	s.updateJob(id, func(job *AsyncJob) {
		job.Status, job.Phase, job.Progress = JobSucceeded, "installed", 100
		job.Result = result
	})
	s.clearActive(id)
}

// outputTail keeps the last n lines of combined manager output.
func outputTail(text string, n int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
