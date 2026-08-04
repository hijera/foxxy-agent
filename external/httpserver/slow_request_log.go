//go:build http

package httpserver

import (
	"net/http"
	"strings"
	"time"
)

// slowRequestThreshold is where a request stops being normal. Everything the SPA opens on boot
// answers in milliseconds; a second is already long enough that a panel feels stuck, and the
// webview only has six connections to spend.
var slowRequestThreshold = 2 * time.Second

// slowStreamFirstByteThreshold is how long a turn may take to produce its first byte before
// that counts as a stall. A model normally starts within a second or two; anything past this
// is the panel sitting on "waiting for the model" with nothing behind it.
var slowStreamFirstByteThreshold = 5 * time.Second

// slowRequestLog reports requests that took long enough for a user to notice.
//
// This exists because the symptom of a stalled panel - "the prompt was sent but nothing
// happens" - is indistinguishable from a slow model, and the difference is only visible from
// the server side.
//
// A streaming turn is judged on **time to first byte**, not total duration: holding the
// connection open for minutes is what a long answer looks like, but taking minutes to emit
// anything at all is the stall being hunted. Judging streams by total duration (as this first
// did) hides exactly the case it was written for.
func (s *Server) slowRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &slowRequestRecorder{ResponseWriter: w, started: started}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(started)

		if rec.streaming() {
			ttfb := rec.firstByteAfter()
			if ttfb < slowStreamFirstByteThreshold {
				return
			}
			s.log.Warn("slow stream start",
				"method", r.Method,
				"path", r.URL.Path,
				"session", strings.TrimSpace(r.Header.Get("X-FoxxyCode-Session-ID")),
				"first_byte_ms", ttfb.Milliseconds(),
				"total_ms", elapsed.Milliseconds(),
			)
			return
		}

		if elapsed < slowRequestThreshold {
			return
		}
		s.log.Warn("slow http request",
			"method", r.Method,
			"path", r.URL.Path,
			"session", strings.TrimSpace(r.Header.Get("X-FoxxyCode-Session-ID")),
			"status", rec.status,
			"ms", elapsed.Milliseconds(),
		)
	})
}

// slowRequestRecorder remembers the status code and whether the response was an event stream.
// It deliberately implements nothing else: Handler() wraps this around the mux, so anything it
// hides from the inner handler (Flusher, Hijacker) would break streaming.
type slowRequestRecorder struct {
	http.ResponseWriter
	status    int
	written   bool
	started   time.Time
	firstByte time.Time
}

// firstByteAfter is how long the handler took to write anything. Zero writes reads as the
// whole request duration, which is the right answer for a turn that produced nothing.
func (r *slowRequestRecorder) firstByteAfter() time.Duration {
	if r.firstByte.IsZero() {
		return time.Since(r.started)
	}
	return r.firstByte.Sub(r.started)
}

func (r *slowRequestRecorder) noteWrite() {
	if r.firstByte.IsZero() {
		r.firstByte = time.Now()
	}
}

func (r *slowRequestRecorder) WriteHeader(code int) {
	if !r.written {
		r.status = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *slowRequestRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.status = http.StatusOK
		r.written = true
	}
	// SSE headers are flushed long before the model says anything, so the first *body* write
	// is what marks the turn actually producing output.
	if len(b) > 0 {
		r.noteWrite()
	}
	return r.ResponseWriter.Write(b)
}

func (r *slowRequestRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *slowRequestRecorder) streaming() bool {
	return strings.HasPrefix(r.Header().Get("Content-Type"), "text/event-stream")
}
