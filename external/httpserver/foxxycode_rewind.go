//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// registerRewindRoute adds the in-place history rewind endpoint.
func (s *Server) registerRewindRoute() {
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/rewind", s.foxxycodeRewind)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/rewind/undo", s.foxxycodeRewindUndo)
}

// foxxycodeRewind handles POST /foxxycode/sessions/{id}/rewind.
//
// Request body:
//
//	{ "userMessageIndex": <int> }
//
// Response:
//
//	{ "object": "foxxycode.session_rewound", "sessionId": "...", "messagesRev": N }
func (s *Server) foxxycodeRewind(w http.ResponseWriter, r *http.Request) {
	// Truncating history is work on the server, so a page on another site must
	// not be able to start it with a "simple" request (see requireJSONRequest).
	if !isJSONRequest(r) {
		writeSubagentsError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}

	var body struct {
		UserMessageIndex *int `json:"userMessageIndex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	// The index is required: a missing one would decode as 0 and wipe
	// everything after the first user message.
	if body.UserMessageIndex == nil {
		http.Error(w, `{"error":{"message":"userMessageIndex is required"}}`, http.StatusBadRequest)
		return
	}
	if *body.UserMessageIndex < 0 {
		http.Error(w, `{"error":{"message":"userMessageIndex must be >= 0"}}`, http.StatusBadRequest)
		return
	}

	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	// A child transcript is read-only on every surface, and so is the session
	// of a scheduler job: rewinding either would rewrite history nobody may
	// change.
	if st.IsReadOnlyTranscript() {
		writeSubagentsError(w, http.StatusConflict, subagentReadOnlyMessage(st))
		return
	}
	// A turn in flight would append onto a history that no longer matches what
	// it was started on; the message queue is empty exactly while no turn runs,
	// so refusing here covers the queued case too. RewindSession then holds the
	// prompt turn lock across the cut, so a turn admitted between this probe
	// and the truncation either already shows in the flag or waits for the lock
	// and runs on the rewound history.
	if s.sessionTurnActive(id) {
		writeSubagentsError(w, http.StatusConflict, "session "+id+" has a turn in flight")
		return
	}

	rev, err := s.mgr.RewindSession(id, *body.UserMessageIndex)
	if err != nil {
		switch {
		case errors.Is(err, session.ErrRewindOutOfRange):
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		case errors.Is(err, session.ErrSessionTurnActive),
			errors.Is(err, session.ErrSubagentReadOnly),
			errors.Is(err, session.ErrSchedulerSessionReadOnly):
			writeSubagentsError(w, http.StatusConflict, err.Error())
		default:
			s.log.Error("rewind session", "session", id, "error", err)
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	s.publishSessionRewound(id, rev)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":      "foxxycode.session_rewound",
		"sessionId":   id,
		"messagesRev": rev,
	})
}

// foxxycodeRewindUndo handles POST /foxxycode/sessions/{id}/rewind/undo.
//
// The JSON body names the rewindUndo.snapshotId from the transcript.
//
// Response:
//
//	{ "object": "foxxycode.session_rewind_undone", "sessionId": "...", "messagesRev": N }
func (s *Server) foxxycodeRewindUndo(w http.ResponseWriter, r *http.Request) {
	if !isJSONRequest(r) {
		writeSubagentsError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}

	var body struct {
		SnapshotID string `json:"snapshotId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.SnapshotID) == "" {
		writeSubagentsError(w, http.StatusBadRequest, "snapshotId is required in a JSON object")
		return
	}
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	// The same read-only transcripts as the rewind: a child session and the
	// session of a scheduler job have no history this surface may rewrite.
	if st.IsReadOnlyTranscript() {
		writeSubagentsError(w, http.StatusConflict, subagentReadOnlyMessage(st))
		return
	}
	rev, err := s.undoRewind(r.Context(), id, body.SnapshotID)
	if err != nil {
		switch {
		case errors.Is(err, session.ErrRewindUndoUnavailable),
			errors.Is(err, session.ErrSessionTurnActive),
			errors.Is(err, session.ErrSubagentReadOnly),
			errors.Is(err, session.ErrSchedulerSessionReadOnly):
			writeSubagentsError(w, http.StatusConflict, err.Error())
		default:
			s.log.Error("rewind undo", "session", id, "error", err)
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	s.publishSessionRewound(id, rev)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":      "foxxycode.session_rewind_undone",
		"sessionId":   id,
		"messagesRev": rev,
	})
}

func (s *Server) undoRewind(ctx context.Context, id, snapshotID string) (uint64, error) {
	rev, err := s.mgr.UndoRewind(id, snapshotID)
	if !errors.Is(err, session.ErrSessionTurnActive) {
		return rev, err
	}
	if err := s.mgr.RequestRewindUndoCancel(id, snapshotID); err != nil {
		// The edited turn may have finished before it was asked to stop.
		if errors.Is(err, session.ErrSessionTurnActive) && !s.sessionTurnActive(id) {
			return s.mgr.UndoRewind(id, snapshotID)
		}
		return 0, err
	}
	deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.Done():
			return 0, session.ErrSessionTurnActive
		case <-ticker.C:
			rev, err := s.mgr.UndoRewind(id, snapshotID)
			if !errors.Is(err, session.ErrSessionTurnActive) {
				return rev, err
			}
		}
	}
}
