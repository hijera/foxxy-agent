//go:build http

package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A streaming turn is judged on time to first byte. Judging it on total duration - which this
// middleware did at first - hides the exact case it exists for: the panel showing "waiting for
// the model" while nothing has been produced. A long answer that starts promptly is normal and
// must stay silent.
func TestSlowStreamIsJudgedOnFirstByte(t *testing.T) {
	restore := slowStreamFirstByteThreshold
	slowStreamFirstByteThreshold = 50 * time.Millisecond
	t.Cleanup(func() { slowStreamFirstByteThreshold = restore })

	cases := []struct {
		name       string
		beforeByte time.Duration
		afterByte  time.Duration
		wantSlow   bool
	}{
		{name: "starts promptly, then streams for a long time", beforeByte: 0, afterByte: 200 * time.Millisecond},
		{name: "produces nothing for a long time", beforeByte: 200 * time.Millisecond, wantSlow: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &slowRequestRecorder{ResponseWriter: httptest.NewRecorder(), started: time.Now()}
			rec.Header().Set("Content-Type", "text/event-stream")
			time.Sleep(tc.beforeByte)
			_, _ = rec.Write([]byte("data: hi\n\n"))
			time.Sleep(tc.afterByte)

			if !rec.streaming() {
				t.Fatal("expected the recorder to see an event stream")
			}
			gotSlow := rec.firstByteAfter() >= slowStreamFirstByteThreshold
			if gotSlow != tc.wantSlow {
				t.Fatalf("slow=%v, want %v (first byte after %s)", gotSlow, tc.wantSlow, rec.firstByteAfter())
			}
		})
	}
}

// A turn that never writes anything is the worst case, and it must not read as instant.
func TestStreamThatNeverWritesCountsAsSlow(t *testing.T) {
	rec := &slowRequestRecorder{ResponseWriter: httptest.NewRecorder(), started: time.Now().Add(-3 * time.Second)}
	rec.Header().Set("Content-Type", "text/event-stream")
	if got := rec.firstByteAfter(); got < 3*time.Second {
		t.Fatalf("a stream with no output reported %s, want at least the request duration", got)
	}
}

// Plain JSON routes keep their own rule: total duration.
func TestNonStreamingRequestKeepsTotalDurationRule(t *testing.T) {
	rec := &slowRequestRecorder{ResponseWriter: httptest.NewRecorder(), started: time.Now()}
	rec.WriteHeader(http.StatusOK)
	if rec.streaming() {
		t.Fatal("a JSON response must not be treated as a stream")
	}
	if rec.status != http.StatusOK {
		t.Fatalf("status = %d", rec.status)
	}
}
