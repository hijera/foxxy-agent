//go:build scheduler

package schedservice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/external/scheduler/storage"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// The fallback walk re-attaches a job session whose sidecar lost its pointer,
// and never a run bundle the old scheduler wrote: those were top-level sched_
// sessions marked with the job id exactly like a job session is.
func TestJobSessionIDForSkipsLegacyRunBundles(t *testing.T) {
	root := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	if err := os.MkdirAll(store.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	schedDir := filepath.Join(root, "scheduler")
	if err := os.MkdirAll(schedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jobPath := filepath.Join(schedDir, "nightly.md")
	if err := os.WriteFile(jobPath, []byte("---\nschedule: \"0 3 * * *\"\n---\nhello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A legacy run: sched_ id, schedulerRun with the job id.
	legacyDir, err := store.EnsureLayout("sched_0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	legacy := &session.State{ID: "sched_0123456789abcdef01234567", CWD: root, Mode: session.ModeAgent, SessionDir: legacyDir}
	legacy.SetSchedulerJobWithoutPersist("nightly")
	if err := store.Save(legacy); err != nil {
		t.Fatal(err)
	}
	if got := JobSessionIDFor(store, jobPath); got != "" {
		t.Fatalf("a legacy run bundle was taken for the job session: %q", got)
	}
	// A real job session is found by the job it names.
	jobSID, err := session.NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := store.EnsureLayout(jobSID)
	if err != nil {
		t.Fatal(err)
	}
	js := &session.State{ID: jobSID, CWD: root, Mode: session.ModeAgent, SessionDir: dir}
	js.SetSchedulerJobWithoutPersist("nightly")
	if err := store.Save(js); err != nil {
		t.Fatal(err)
	}
	if got := JobSessionIDFor(store, jobPath); got != jobSID {
		t.Fatalf("job session = %q, want %q", got, jobSID)
	}
	// The sidecar pointer wins over the walk.
	if err := storage.WriteJobSessionID(storage.StatePath(jobPath), "sess_fedcba9876543210fedcba98"); err != nil {
		t.Fatal(err)
	}
	if got := JobSessionIDFor(store, jobPath); got != "sess_fedcba9876543210fedcba98" {
		t.Fatalf("pointer ignored: %q", got)
	}
}

func TestListJobRunsIncludesLegacyBundles(t *testing.T) {
	root := t.TempDir()
	schedDir := filepath.Join(root, "scheduler")
	if err := os.MkdirAll(schedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jobPath := filepath.Join(schedDir, "nightly.md")
	if err := os.WriteFile(jobPath, []byte("---\nschedule: \"0 3 * * *\"\n---\nhello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	legacyID := "sched_0123456789abcdef01234567"
	legacyDir, err := store.EnsureLayout(legacyID)
	if err != nil {
		t.Fatal(err)
	}
	state := &session.State{ID: legacyID, CWD: root, Mode: session.ModeAgent, SessionDir: legacyDir}
	state.SetSchedulerJobWithoutPersist("nightly")
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	meta, err := store.ReadMeta(legacyID)
	if err != nil {
		t.Fatal(err)
	}
	meta.SchedulerStartedAt = "2026-09-01T03:00:00Z"
	meta.SchedulerEndedAt = "2026-09-01T03:01:00Z"
	meta.SchedulerStopStatus = "completed"
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "session.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.Paths{Home: root}, Scheduler: config.SchedulerConfig{Enabled: true, Dir: schedDir}}
	svc := NewService(cfg, nil, root)
	rows, err := svc.ListJobRuns("nightly", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].SessionID != legacyID || rows[0].Status != "succeeded" || rows[0].ElapsedSeconds != 60 {
		t.Fatalf("legacy run: %+v", rows)
	}
	// Saving an old run with the new persistence code must not erase its
	// historical status and timestamps.
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	meta, err = store.ReadMeta(legacyID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchedulerStopStatus != "completed" || meta.SchedulerStartedAt == "" {
		t.Fatalf("legacy metadata was lost: %+v", meta)
	}
}
