//go:build http

package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// registerQueueRoutes wires the per-session message queue: what an operator
// writes while a turn is running, waiting for that turn to read it at its next
// step (docs/features/message-queue.md).
//
// The queue lives in process memory. Admission opens during a turn; an
// after_turn message may remain visible after Stop until it is removed or run.
func (s *Server) registerQueueRoutes() {
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/queue", s.foxxycodeQueueList)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/queue", s.foxxycodeQueuePost)
	s.mux.HandleFunc("DELETE /foxxycode/sessions/{id}/queue", s.foxxycodeQueueClear)
	s.mux.HandleFunc("DELETE /foxxycode/sessions/{id}/queue/{message_id}", s.foxxycodeQueueDelete)
	s.mux.HandleFunc("PATCH /foxxycode/sessions/{id}/queue/{message_id}", s.foxxycodeQueuePatch)
}

// queueBody is what a client posts to add a follow-up.
type queueBody struct {
	Text        string             `json:"text"`
	Mode        session.QueueMode  `json:"mode,omitempty"`
	InlineFiles []acp.ImagePartRef `json:"inline_files,omitempty"`
}

// writeQueue answers with the queue as it now stands.
//
// The answer carries the same version the SSE frames do, so a client applying
// both keeps whichever is newer rather than letting a request that finished
// late overwrite a change it already heard about.
//
// message is the one message the request was about, when there is one: the
// row it added, or the row it took back (takenBack). imagesDropped counts the
// attachments the request was refused to carry to a model that takes no images;
// it is named only when there were any, so the client can say the drop happened
// rather than showing a queue row that silently lost them. notice says what
// the settings commands at the start of the text changed - also when a message
// was queued with them, so a model switch applied that way is never silent.
func writeQueue(w http.ResponseWriter, status int, sessionID string, st *session.State, message interface{}, imagesDropped int, notice string) {
	queue, version := st.QueueSnapshot()
	out := map[string]interface{}{
		"object":    "foxxycode.message_queue",
		"sessionId": sessionID,
		"messages":  session.QueuedMessagesWire(queue),
		"version":   version,
	}
	if message != nil {
		out["message"] = message
	}
	if imagesDropped > 0 {
		out["images_dropped"] = imagesDropped
	}
	if strings.TrimSpace(notice) != "" {
		out["notice"] = notice
	}
	writeJSON(w, status, out)
}

// takenBackMessage is a message the operator took back, with everything it
// carried: inline_files holds its images in full, in the shape POST takes, so a
// client can put the message back into its draft exactly as it was written.
// Rows everywhere else name their images and never carry them.
type takenBackMessage struct {
	acp.QueuedMessage
	InlineFiles []acp.ImagePartRef `json:"inline_files,omitempty"`
}

func takenBack(q session.QueuedMessage) takenBackMessage {
	return takenBackMessage{QueuedMessage: q.Wire(), InlineFiles: q.ImageParts}
}

// queueInlineFiles checks the files a follow-up brings. They are kept in
// memory until the agent reads the message, then saved with the session's
// assets and sent to the model, so only a file carried in the request itself
// is taken: a base64 data URI of its media type - an image or a document, the
// same set POST /v1/responses forwards from the composer's picker - never a
// URL to fetch.
func queueInlineFiles(files []acp.ImagePartRef) ([]acp.ImagePartRef, error) {
	for i, f := range files {
		header, payload, ok := strings.Cut(f.DataURL, ",")
		h := strings.ToLower(header)
		media, isBase64 := strings.CutSuffix(h, ";base64")
		if !ok || !isBase64 || !strings.HasPrefix(media, "data:") || len(media) == len("data:") || payload == "" {
			return nil, fmt.Errorf("inline_files[%d]: data_url must be a base64 data URI of the file (data:<media type>;base64,...)", i)
		}
	}
	return files, nil
}

// queueSession resolves the session a queue route names, loading a persisted
// bundle the way every other /foxxycode route does.
func (s *Server) queueSession(w http.ResponseWriter, r *http.Request) (string, *session.State) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return "", nil
	}
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return "", nil
	}
	return id, st
}

func (s *Server) foxxycodeQueueList(w http.ResponseWriter, r *http.Request) {
	id, st := s.queueSession(w, r)
	if st == nil {
		return
	}
	writeQueue(w, http.StatusOK, id, st, nil, 0, "")
}

func (s *Server) foxxycodeQueuePost(w http.ResponseWriter, r *http.Request) {
	id, st := s.queueSession(w, r)
	if st == nil {
		return
	}
	var body queueBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON body"}}`, http.StatusBadRequest)
		return
	}
	// Settings commands at the start of the text apply at once and never
	// reach the model; only the rest is queued (session.EnqueueFollowUp).
	if body.Mode == "" {
		body.Mode = session.QueueModeSteer
	}
	files, err := queueInlineFiles(body.InlineFiles)
	if err != nil {
		s.queueError(w, http.StatusBadRequest, "invalid_request", err)
		return
	}
	// Images reach the model only when it reads them - the rule POST
	// /v1/responses applies to an ordinary prompt. The drop is never silent: a
	// body whose attachments were all it carried has nothing left to say and is
	// refused so the client keeps them, and one that still has text answers with
	// the number it dropped.
	var imagesDropped int
	if cfg := s.activeCfg(); len(files) > 0 && !configuredModelMultimodal(cfg, effectiveYAMLModel(cfg, st)) {
		imagesDropped = len(files)
		files = nil
		if strings.TrimSpace(body.Text) == "" {
			s.queueError(w, http.StatusBadRequest, "model_not_multimodal", fmt.Errorf(
				"the session's model takes no images, so its %d attachment(s) cannot be queued: switch the model or send the message without them", imagesDropped))
			return
		}
	}
	msg, queued, notice, err := s.mgr.EnqueueFollowUpWithMode(r.Context(), id, body.Text, "web", body.Mode, files)
	switch {
	case errors.Is(err, session.ErrTurnScopedFollowUp):
		s.queueError(w, http.StatusConflict, "turn_scoped_follow_up", err)
		return
	case errors.Is(err, session.ErrNoActiveTurn):
		// Another FoxxyCode process over the same home (a second IDE window,
		// the console) may be running this session's turn. Its queue lives in
		// that process, out of this server's reach, and the prompt a client
		// falls back to below would only wait for the turn lock and be refused
		// as busy - so the answer is busy now, and the client keeps the text.
		if s.sessionTurnActive(id) {
			writeSessionBusy(w, id, "session busy: another FoxxyCode process is running this turn, and its queue is there")
			return
		}
		// 409 rather than 400: the request is well formed, the session is
		// simply not working right now. The client sends it as an ordinary
		// prompt instead, which is what the SPA and the console both do.
		s.queueError(w, http.StatusConflict, "no_active_turn", err)
		return
	case errors.Is(err, session.ErrQueueFull):
		s.queueError(w, http.StatusConflict, "queue_full", err)
		return
	case isSubagentReadOnly(err):
		s.queueError(w, http.StatusConflict, "subagent_read_only", err)
		return
	case err != nil:
		s.queueError(w, http.StatusBadRequest, "invalid_request", err)
		return
	}
	if !queued {
		// Only settings commands: nothing waits for the turn, the notice
		// says what changed.
		out := map[string]interface{}{
			"object":    "foxxycode.message_queue",
			"sessionId": id,
			"notice":    notice,
		}
		queue, version := st.QueueSnapshot()
		out["messages"] = session.QueuedMessagesWire(queue)
		out["version"] = version
		if imagesDropped > 0 {
			out["images_dropped"] = imagesDropped
		}
		if snap, err := s.mgr.SessionSettings(id); err == nil {
			out["settings"] = snap
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeQueue(w, http.StatusCreated, id, st, msg.Wire(), imagesDropped, notice)
}

func (s *Server) foxxycodeQueuePatch(w http.ResponseWriter, r *http.Request) {
	id, st := s.queueSession(w, r)
	if st == nil {
		return
	}
	var body struct {
		Mode session.QueueMode `json:"mode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil || !session.ValidQueueMode(body.Mode) {
		s.queueError(w, http.StatusBadRequest, "invalid_request", fmt.Errorf("mode must be steer or after_turn"))
		return
	}
	_, err := s.mgr.SetQueuedTurnMessageMode(id, strings.TrimSpace(r.PathValue("message_id")), body.Mode)
	if errors.Is(err, session.ErrQueuedMessageNotFound) {
		s.queueError(w, http.StatusNotFound, "not_found", err)
		return
	}
	if err != nil {
		s.queueError(w, http.StatusBadRequest, "invalid_request", err)
		return
	}
	writeQueue(w, http.StatusOK, id, st, nil, 0, "")
}

func (s *Server) foxxycodeQueueDelete(w http.ResponseWriter, r *http.Request) {
	id, st := s.queueSession(w, r)
	if st == nil {
		return
	}
	messageID := strings.TrimSpace(r.PathValue("message_id"))
	taken, _, err := s.mgr.TakeBackQueuedTurnMessage(id, messageID)
	switch {
	case errors.Is(err, session.ErrQueuedMessageNotFound):
		// Losing the race with the agent is the ordinary way this happens: the
		// message was read a moment ago and is now part of the conversation.
		s.queueError(w, http.StatusNotFound, "not_found", err)
		return
	case err != nil:
		s.queueError(w, http.StatusBadRequest, "invalid_request", err)
		return
	}
	writeQueue(w, http.StatusOK, id, st, takenBack(taken), 0, "")
}

func (s *Server) foxxycodeQueueClear(w http.ResponseWriter, r *http.Request) {
	id, st := s.queueSession(w, r)
	if st == nil {
		return
	}
	if err := s.mgr.ClearQueuedTurnMessages(id); err != nil {
		s.queueError(w, http.StatusBadRequest, "invalid_request", err)
		return
	}
	writeQueue(w, http.StatusOK, id, st, nil, 0, "")
}

// queueError answers in the error shape the rest of /foxxycode uses, with a code a
// client can branch on rather than matching prose.
func (s *Server) queueError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"message": err.Error(),
			"code":    code,
		},
	})
}
