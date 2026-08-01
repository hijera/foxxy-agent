//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// registerBackgroundRoutes wires the background task surface the tasks panel
// polls. The pool is process-wide, so these handlers answer for whatever the
// agent loop started, including tasks whose turn already ended.
func (s *Server) registerBackgroundRoutes() {
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/background-tasks", s.foxxycodeBackgroundTasksList)
	s.mux.HandleFunc("DELETE /foxxycode/sessions/{id}/background-tasks", s.foxxycodeBackgroundTasksClear)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/background-tasks/{task_id}", s.foxxycodeBackgroundTaskGet)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/background-tasks/{task_id}/stop", s.foxxycodeBackgroundTaskStop)
}

// wakeBusyRetryBudget bounds how long a wake keeps waiting for a session whose
// turn lock is held. A user turn can legitimately run for minutes, and the whole
// point of notify_on_finish is that the outcome is not lost, so the retry has to
// outlast an ordinary turn. It still has to end: a session left permanently busy
// must not pin a goroutine for the life of the process.
const wakeBusyRetryBudget = 10 * time.Minute

// wakeBusyRetryMax caps the backoff between attempts. Small enough that the wake
// lands promptly once the session frees up.
const wakeBusyRetryMax = 15 * time.Second

// attachBackgroundWaker lets a finished task that asked for it start an
// autonomous turn, which is what makes a session usable while nobody watches
// it. The turn goes through the manager's normal prompt path, so it takes the
// session turn lock.
func (s *Server) attachBackgroundWaker() {
	waker := agent.NewBackgroundWaker(s.log, func(ctx context.Context, sessionID, instruction string) error {
		if bgtask.Default().Draining() {
			return nil
		}
		if s.mgr.SessionByID(sessionID) == nil {
			if _, err := s.mgr.HandleSessionLoad(ctx, acp.SessionLoadParams{
				SessionID: sessionID,
				CWD:       s.defaultCWD,
			}); err != nil {
				return err
			}
		}
		s.bgWG.Add(1)
		defer s.bgWG.Done()
		return s.runWakeTurn(ctx, sessionID, instruction)
	})
	waker.Attach(bgtask.Default())
}

// runWakeTurn prompts the session, retrying while the turn lock is held.
//
// This is where the fork diverges from upstream. Upstream's manager queues on
// the lock, so a wake that lands mid-turn simply waits. Ours fails fast with
// ErrSessionTurnBusy -- the same behaviour that lets the composer answer 409
// session_busy instead of hanging -- and the waker drops a failed batch, so a
// task finishing while the user is mid-turn would lose its notification
// entirely. Retrying here keeps that guarantee without giving the lock a
// queue, which the SPA relies on not having.
func (s *Server) runWakeTurn(ctx context.Context, sessionID, instruction string) error {
	params := acp.SessionPromptParams{
		SessionID: sessionID,
		Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: instruction}},
	}

	deadline := time.Now().Add(wakeBusyRetryBudget)
	backoff := time.Second
	for {
		_, err := s.mgr.HandleSessionPromptWithSender(ctx, params, planRunNoopSender{}, nil)
		if !errors.Is(err, session.ErrSessionTurnBusy) {
			return err
		}
		if time.Now().After(deadline) {
			return err
		}

		s.log.Info("background_wake_busy_retry",
			"session_id", sessionID, "retry_in", backoff.String())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// Shutdown began while we were waiting: the turn would be cancelled
		// immediately and nobody would read it.
		if bgtask.Default().Draining() {
			return nil
		}
		if backoff *= 2; backoff > wakeBusyRetryMax {
			backoff = wakeBusyRetryMax
		}
	}
}

func (s *Server) foxxycodeBackgroundTasksClear(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	if dir := strings.TrimSpace(st.GetPersistedSessionDir()); dir != "" {
		bgtask.Default().SetSessionDir(id, dir)
	}

	cleared := bgtask.Default().ClearFinished(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.background_tasks_cleared",
		"sessionId": id,
		"cleared":   cleared,
	})
}

// backgroundTaskRow is the JSON shape the SPA renders. Elapsed and overdue are
// computed server-side so every client agrees on them without re-deriving the
// clock arithmetic.
type backgroundTaskRow struct {
	bgtask.Snapshot
	ElapsedSeconds int  `json:"elapsed_seconds"`
	Overdue        bool `json:"overdue"`
	Running        bool `json:"running"`
}

func newBackgroundTaskRow(snap bgtask.Snapshot, now time.Time) backgroundTaskRow {
	return backgroundTaskRow{
		Snapshot:       snap,
		ElapsedSeconds: int(snap.Elapsed(now) / time.Second),
		Overdue:        snap.Overdue(now),
		Running:        !snap.Status.Finished(),
	}
}

// backgroundRowsForSession merges the live pool with what the session bundle
// recorded, so tasks from a previous process still appear (as orphaned) instead
// of vanishing from the drawer after a restart.
func backgroundRowsForSession(sessionID, sessionDir string, now time.Time) []backgroundTaskRow {
	live := bgtask.Default().List(sessionID)
	seen := make(map[string]bool, len(live))
	rows := make([]backgroundTaskRow, 0, len(live))
	for _, snap := range live {
		seen[snap.ID] = true
		rows = append(rows, newBackgroundTaskRow(snap, now))
	}

	for _, snap := range bgtask.LoadPersisted(sessionDir) {
		if seen[snap.ID] {
			continue
		}
		snap.SessionID = sessionID
		rows = append(rows, newBackgroundTaskRow(snap, now))
	}
	return rows
}

func (s *Server) foxxycodeBackgroundTasksList(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}

	now := time.Now()
	rows := backgroundRowsForSession(id, strings.TrimSpace(st.GetPersistedSessionDir()), now)
	running := 0
	for _, row := range rows {
		if row.Running {
			running++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.background_task_list",
		"sessionId": id,
		"running":   running,
		"data":      rows,
	})
}

func (s *Server) foxxycodeBackgroundTaskGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	taskID := strings.TrimSpace(r.PathValue("task_id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	sessionDir := strings.TrimSpace(st.GetPersistedSessionDir())
	now := time.Now()

	tail := 0
	if v := strings.TrimSpace(r.URL.Query().Get("tail")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			http.Error(w, `{"error":{"message":"tail must be a non-negative integer"}}`, http.StatusBadRequest)
			return
		}
		tail = n
	}

	output, snap, err := bgtask.Default().Output(id, taskID, tail)
	if err != nil {
		// The pool forgets tasks from an earlier process; the session bundle
		// still has the record and the log.
		row, ok := findPersistedTask(sessionDir, taskID)
		if !ok {
			http.Error(w, `{"error":{"message":"background task not found"}}`, http.StatusNotFound)
			return
		}
		row.SessionID = id
		persisted, dropped, _ := bgtask.PersistedOutput(sessionDir, taskID)
		row.OutputTruncated = row.OutputTruncated || dropped
		writeBackgroundTask(w, id, newBackgroundTaskRow(row, now), tailLines(persisted, tail))
		return
	}

	writeBackgroundTask(w, id, newBackgroundTaskRow(snap, now), output)
}

func (s *Server) foxxycodeBackgroundTaskStop(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	taskID := strings.TrimSpace(r.PathValue("task_id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}

	snap, err := bgtask.Default().Stop(id, taskID)
	if err != nil {
		// An unknown id is a 404; a task that exists but could not be
		// terminated is a server-side failure and must not read as "no such
		// task", which would tell the operator the process is gone.
		if errors.Is(err, bgtask.ErrNotFound) {
			http.Error(w, `{"error":{"message":"background task not found"}}`, http.StatusNotFound)
			return
		}
		s.log.Error("background task stop", "session", id, "task", taskID, "error", err)
		http.Error(w, `{"error":{"message":"could not stop the background task"}}`, http.StatusInternalServerError)
		return
	}

	output, _, _ := bgtask.Default().Output(id, taskID, 0)
	writeBackgroundTask(w, id, newBackgroundTaskRow(snap, time.Now()), output)
}

func writeBackgroundTask(w http.ResponseWriter, sessionID string, row backgroundTaskRow, output string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.background_task",
		"sessionId": sessionID,
		"task":      row,
		"output":    output,
	})
}

func findPersistedTask(sessionDir, taskID string) (bgtask.Snapshot, bool) {
	for _, snap := range bgtask.LoadPersisted(sessionDir) {
		if snap.ID == taskID {
			return snap, true
		}
	}
	return bgtask.Snapshot{}, false
}

// tailLines trims text to its last n lines, matching what the pool does for a
// live task so both paths answer the same shape.
func tailLines(text string, n int) string {
	if n <= 0 || text == "" {
		return text
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
