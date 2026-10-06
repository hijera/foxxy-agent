//go:build cli

package cli

import (
	"strings"
	"testing"
)

// The fork's turn lock fails fast: a prompt posted while another client owns
// the turn is answered 409 session_busy instead of waiting its turn. When the
// console has not heard of that turn yet, the refused prompt must end up where
// it would have gone had the activity update arrived first - in the queue -
// and not be lost behind a "Turn failed" line.
func TestRemoteControlsBusyPromptIsTakenBackAndQueued(t *testing.T) {
	f := newRemoteControlStand(t)
	f.mu.Lock()
	f.active = true // the server owns a turn; no turn_started frame was delivered
	f.mu.Unlock()

	f.app.editor.SetText("also cover the Windows path")
	f.app.dispatchInput([]byte("\r"))
	f.request(t, "prompt")
	// The stand has no UI loop: the worker's turnDone is applied by hand, and
	// that is the message the refusal is handled in.
	pumpControls(t, f.app, func(msg updateMsg) bool { _, done := msg.update.(turnDone); return done })
	if got := f.request(t, "queue"); got.text != "also cover the Windows path" {
		t.Fatalf("queued text = %q", got.text)
	}
	pumpControls(t, f.app, func(updateMsg) bool { return len(f.app.queue.Rows()) == 1 })

	if f.app.turnActive {
		t.Fatal("a refused prompt left a local turn running")
	}
	screen := transcriptText(f.app)
	if strings.Contains(screen, "Turn failed") {
		t.Fatalf("a busy session must not read as a failed turn:\n%s", screen)
	}
	if strings.Contains(screen, "also cover the Windows path") {
		t.Fatalf("the refused prompt stayed in the transcript next to its queued copy:\n%s", screen)
	}
}
