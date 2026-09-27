//go:build gateway || gateway.telegram

package telegram

import (
	"strings"
	"testing"
	"time"
)

// fork(stop-notice-transcript) guard: the agent streams the notice that says
// why a turn stopped short into the answer, so the chat reads it once, inside
// the answer. Upstream 1.2.9 also replies with StopNotice as a message of its
// own; here that would say it twice.
func TestTelegramDoesNotRepeatTheStopNotice(t *testing.T) {
	const notice = "Stopped after 30 steps, the step limit set by agent.max_turns."
	w := &pollingWorld{}
	defer w.close()
	for _, step := range []func() error{
		func() error { return w.fakeBotAPI("foxxycode_fake_bot") },
		w.gatewayPointedAtIt,
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	w.runner.mu.Lock()
	w.runner.answer = "Partial work.\n\n" + notice
	w.runner.stopNotice = notice
	w.runner.mu.Unlock()
	if err := w.botStarted(); err != nil {
		t.Fatal(err)
	}
	if err := w.userSends("hello"); err != nil {
		t.Fatal(err)
	}
	if err := w.chatShowsBotMessage("Partial work."); err != nil {
		t.Fatal(err)
	}
	// Give a reply of its own the time it would take to arrive.
	time.Sleep(400 * time.Millisecond)
	count := 0
	for _, m := range w.fake.Chat(pollingChatID).Messages {
		if m.From == "bot" && !m.Deleted {
			count += strings.Count(m.Text, notice)
		}
	}
	if count != 1 {
		t.Fatalf("the chat shows the notice %d times, want once:\n%s", count, w.fake.Chat(pollingChatID).Text())
	}
}
