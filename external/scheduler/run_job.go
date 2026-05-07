//go:build scheduler

package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	sched "github.com/EvilFreelancer/coddy-agent/external/scheduler/lib"
	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	"github.com/EvilFreelancer/coddy-agent/internal/agent"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
	"github.com/EvilFreelancer/coddy-agent/internal/skills"
)

func randomSchedulerSessionID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "sched_err"
	}
	return "sched_" + hex.EncodeToString(b)
}

func resolveJobCWD(processCWD string, fm *sched.JobFrontmatter) (string, error) {
	base := strings.TrimSpace(processCWD)
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		base = wd
	}
	raw := strings.TrimSpace(fm.CWD)
	if raw == "" {
		return filepath.Clean(base), nil
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	return filepath.Clean(filepath.Join(base, raw)), nil
}

func parseSessionMode(s string) session.Mode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "agent":
		return session.ModeAgent
	case "plan":
		return session.ModePlan
	default:
		return session.ModeAgent
	}
}

func runJobFile(ctx context.Context, cfg *config.Config, log *slog.Logger, processCWD, jobPath string, fireSlot time.Time, fm *sched.JobFrontmatter, instruction string) error {
	lock := sched.LockPath(jobPath)
	stPath := sched.StatePath(jobPath)

	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	_, _ = f.WriteString(time.Now().UTC().Format(time.RFC3339) + "\n")
	_ = f.Close()
	defer func() { _ = os.Remove(lock) }()

	log.Info("scheduler job start", "utc", time.Now().UTC().Format(time.RFC3339), "job", jobPath)

	jobCWD, err := resolveJobCWD(processCWD, fm)
	if err != nil {
		log.Error("scheduler job cwd", "job", jobPath, "error", err)
		return err
	}

	loader := skills.NewLoader(cfg.Skills.Dirs)
	skillList, err := loader.LoadAll(jobCWD, cfg.Paths.Home)
	if err != nil {
		log.Warn("scheduler skills load", "error", err)
		skillList = nil
	}

	st := &session.State{
		ID:              randomSchedulerSessionID(),
		CWD:             jobCWD,
		Mode:            parseSessionMode(fm.Mode),
		SelectedModelID: strings.TrimSpace(fm.Model),
		Skills:          skillList,
	}

	timeout, err := time.ParseDuration(cfg.Scheduler.Timeout)
	if err != nil {
		timeout = 30 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var snd autoAllowSender
	text, stop, runErr := agent.RunScheduledTurn(runCtx, cfg, st, log, snd, instruction)
	if runErr != nil {
		log.Error("scheduler job failed", "job", jobPath, "stop", stop, "error", runErr)
	} else {
		const maxLog = 16000
		out := text
		if len(out) > maxLog {
			out = out[:maxLog] + fmt.Sprintf(" ... (%d bytes truncated)", len(text))
		}
		log.Info("scheduler job done", "job", jobPath, "stop", stop, "assistant_chars", len(text), "output", out)
	}

	if werr := sched.WriteJobState(stPath, fireSlot); werr != nil {
		log.Warn("scheduler state write", "path", stPath, "error", werr)
		if runErr == nil {
			return werr
		}
	}
	return runErr
}

// autoAllowSender drops streamed updates and allows every permission request (unattended scheduler runs).
type autoAllowSender struct{}

func (autoAllowSender) SendSessionUpdate(string, interface{}) error { return nil }

func (autoAllowSender) RequestPermission(_ context.Context, _ acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	return &acp.PermissionResult{Outcome: "allow", OptionID: "allow"}, nil
}
