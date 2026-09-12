package session

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// MessageCount is what a client polling a long turn watches: activitySeq only
// advances once a turn has completed, so it cannot say whether the transcript
// grew while the turn is still running.
func TestMessageCountTracksTheTranscriptWithinATurn(t *testing.T) {
	st := &State{ID: "sess_count"}
	if got := st.MessageCount(); got != 0 {
		t.Fatalf("empty transcript reports %d", got)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "hello"})
	if got := st.MessageCount(); got != 2 {
		t.Fatalf("MessageCount = %d, want 2", got)
	}
	// It counts what GetMessages would return, without paying for the copy.
	if got, want := st.MessageCount(), len(st.GetMessages()); got != want {
		t.Fatalf("MessageCount = %d, GetMessages = %d", got, want)
	}
	// A completed turn is what moves activitySeq; the message counter does not
	// care, which is exactly why both exist.
	before := st.GetActivitySeq()
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "more"})
	if st.GetActivitySeq() != before {
		t.Fatal("adding a message moved activitySeq")
	}
	if got := st.MessageCount(); got != 3 {
		t.Fatalf("MessageCount = %d, want 3", got)
	}
}
