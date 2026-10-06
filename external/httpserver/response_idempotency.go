//go:build http

package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// handleResponsesCreate claims a logical submission before any session/model mutation.
func (s *Server) handleResponsesCreate(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		s.handleResponsesCreateOnce(w, r)
		return
	}
	if len(key) > 128 {
		http.Error(w, `{"error":{"message":"Idempotency-Key exceeds 128 bytes"}}`, http.StatusBadRequest)
		return
	}
	fs := s.mgr.FileStore()
	if fs == nil || fs.Root == "" {
		http.Error(w, `{"error":{"message":"idempotency requires a session store"}}`, http.StatusServiceUnavailable)
		return
	}
	const maxRequestBytes = 32 << 20
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		http.Error(w, `{"error":{"message":"request body unavailable or exceeds 32 MiB"}}`, http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	sid := strings.TrimSpace(r.Header.Get("X-FoxxyCode-Session-ID"))
	fingerprint := session.ObservationHash(r.URL.Path, sid, string(body))
	receipt, claimed, err := fs.ClaimResponseRequest(key, fingerprint, sid)
	if err != nil {
		if errors.Is(err, session.ErrResponseRequestConflict) {
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusConflict)
			return
		}
		s.log.Error("response request claim", "error", err)
		http.Error(w, `{"error":{"message":"request receipt unavailable; retry the same key"}}`, http.StatusServiceUnavailable)
		return
	}
	if !claimed {
		w.Header().Set("Idempotency-Replayed", "true")
		if receipt.Completed {
			f, err := receipt.OpenBody()
			if err != nil {
				http.Error(w, `{"error":{"message":"saved response unavailable"}}`, http.StatusServiceUnavailable)
				return
			}
			defer func() { _ = f.Close() }()
			for k, v := range receipt.Headers {
				w.Header()[k] = v
			}
			w.WriteHeader(receipt.Code)
			_, _ = io.Copy(w, f)
			return
		}
		status := "interrupted"
		if _, ok := s.responseRequests.Load(key); ok {
			status = "in_progress"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": receipt.SessionID, "object": "response", "status": status})
		return
	}
	s.responseRequests.Store(key, true)
	defer s.responseRequests.Delete(key)
	file, err := receipt.CreateBody()
	if err != nil {
		http.Error(w, `{"error":{"message":"cannot persist response"}}`, http.StatusServiceUnavailable)
		return
	}
	defer func() { _ = file.Close() }()
	recorder := &receiptWriter{ResponseWriter: w, file: file, receipt: receipt}
	s.handleResponsesCreateOnce(recorder, r)
	if recorder.code == 0 {
		recorder.WriteHeader(http.StatusOK)
	}
	if err := file.Sync(); err != nil {
		recorder.saveErr = err
	}
	if recorder.saveErr == nil {
		receipt.Completed = true
		if err := receipt.Save(); err != nil {
			s.log.Error("complete response receipt", "error", err)
		}
	} else {
		s.log.Error("persist response body", "error", recorder.saveErr)
	}
}

// receiptWriter spools the complete wire output even when the original client disconnects.
// Only a fully saved body is marked replayable; partial receipts remain interrupted.
type receiptWriter struct {
	http.ResponseWriter
	file    *os.File
	receipt *session.ResponseReceipt
	code    int
	saveErr error
}

func (w *receiptWriter) WriteHeader(code int) {
	if w.code != 0 {
		return
	}
	w.code = code
	w.receipt.Code = code
	w.receipt.Headers = w.Header().Clone()
	// CORS is evaluated for each caller, never replayed from a previous origin.
	for name := range w.receipt.Headers {
		if strings.HasPrefix(strings.ToLower(name), "access-control-") || strings.EqualFold(name, "Vary") {
			delete(w.receipt.Headers, name)
		}
	}
	if sid := w.Header().Get("X-FoxxyCode-Session-ID"); sid != "" {
		w.receipt.SessionID = sid
	}
	w.saveErr = w.receipt.Save()
	w.ResponseWriter.WriteHeader(code)
}
func (w *receiptWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if _, err := w.file.Write(b); err != nil {
		w.saveErr = err
	}
	return w.ResponseWriter.Write(b)
}
func (w *receiptWriter) Flush() {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
