//go:build scheduler

package schedservice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxy-agent/external/scheduler/storage"
	"github.com/hijera/foxxy-agent/internal/config"
)

func TestPatchJobRenameJobID(t *testing.T) {
	root := t.TempDir()
	schedDir := filepath.Join(root, "scheduler")
	if err := os.MkdirAll(schedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Scheduler: config.SchedulerConfig{Enabled: true, Dir: schedDir},
	}
	cfg.Scheduler.Normalize(config.Paths{CWD: root})
	cfg.Scheduler.ApplyDefaults(config.Paths{CWD: root})

	svc := NewService(cfg, nil, root)
	if err := svc.CreateJob(SchedulerJobCreate{
		JobID:       "old-name",
		Description: "Before rename",
		Schedule:    "0 * * * *",
		Body:        "tick",
	}); err != nil {
		t.Fatal(err)
	}
	state := []byte(`{"last_scheduled_utc":"2026-05-01T12:00:00Z"}`)
	oldAbs, _ := svc.jobAbsPath("old-name")
	if err := os.WriteFile(storage.StatePath(oldAbs), state, 0o644); err != nil {
		t.Fatal(err)
	}

	newID := "new-name"
	if err := svc.PatchJob("old-name", SchedulerJobPatch{JobID: &newID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldAbs); !os.IsNotExist(err) {
		t.Fatalf("old .md should be gone: %v", err)
	}
	newAbs, err := svc.jobAbsPath("new-name")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newAbs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storage.StatePath(newAbs)); err != nil {
		t.Fatalf("state sidecar should move with rename: %v", err)
	}
	got, err := svc.GetJob("new-name")
	if err != nil {
		t.Fatal(err)
	}
	if got.JobID != "new-name" || got.Description != "Before rename" {
		t.Fatalf("got %+v", got)
	}
}

func TestPatchJobRenameJobIDConflict(t *testing.T) {
	root := t.TempDir()
	schedDir := filepath.Join(root, "scheduler")
	if err := os.MkdirAll(schedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Scheduler: config.SchedulerConfig{Enabled: true, Dir: schedDir},
	}
	cfg.Scheduler.Normalize(config.Paths{CWD: root})
	cfg.Scheduler.ApplyDefaults(config.Paths{CWD: root})
	svc := NewService(cfg, nil, root)
	for _, id := range []string{"taken", "mover"} {
		if err := svc.CreateJob(SchedulerJobCreate{
			JobID: id, Description: "x", Schedule: "0 * * * *", Body: "y",
		}); err != nil {
			t.Fatal(err)
		}
	}
	target := "taken"
	if err := svc.PatchJob("mover", SchedulerJobPatch{JobID: &target}); err != ErrJobExists {
		t.Fatalf("want ErrJobExists, got %v", err)
	}
}

func TestDeleteJobBlockedByLock(t *testing.T) {
	root := t.TempDir()
	schedDir := filepath.Join(root, "scheduler")
	if err := os.MkdirAll(schedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Scheduler: config.SchedulerConfig{Enabled: true, Dir: schedDir},
	}
	cfg.Scheduler.Normalize(config.Paths{CWD: root})
	cfg.Scheduler.ApplyDefaults(config.Paths{CWD: root})
	svc := NewService(cfg, nil, root)
	if err := svc.CreateJob(SchedulerJobCreate{
		JobID: "locked", Description: "x", Schedule: "0 * * * *", Body: "y",
	}); err != nil {
		t.Fatal(err)
	}
	abs, err := svc.jobAbsPath("locked")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storage.LockPath(abs), []byte("2026-05-01T12:00:00Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteJob("locked"); err != ErrJobBusy {
		t.Fatalf("want ErrJobBusy, got %v", err)
	}
}

func TestPatchJobRenameRejectsInvalidID(t *testing.T) {
	root := t.TempDir()
	schedDir := filepath.Join(root, "scheduler")
	if err := os.MkdirAll(schedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Scheduler: config.SchedulerConfig{Enabled: true, Dir: schedDir},
	}
	cfg.Scheduler.Normalize(config.Paths{CWD: root})
	cfg.Scheduler.ApplyDefaults(config.Paths{CWD: root})
	svc := NewService(cfg, nil, root)
	if err := svc.CreateJob(SchedulerJobCreate{
		JobID: "ok", Description: "x", Schedule: "0 * * * *", Body: "y",
	}); err != nil {
		t.Fatal(err)
	}
	bad := "bad/id"
	renameErr := svc.PatchJob("ok", SchedulerJobPatch{JobID: &bad})
	if renameErr == nil {
		t.Fatal("want invalid job_id error")
	}
	if renameErr != ErrInvalidJobID && !strings.Contains(renameErr.Error(), "invalid") {
		t.Fatalf("got %v", renameErr)
	}
}
