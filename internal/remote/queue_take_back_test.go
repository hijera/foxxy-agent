package remote

import (
	"io"
	"net/http"
	"testing"
)

func TestRemoteTakeBackQueueMessageRestoresNestedInlineFiles(t *testing.T) {
	h, _ := controlHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/foxxycode/sessions/sess_shared/queue/q_1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"message":{"id":"q_1","text":"look","mode":"after_turn","imageParts":[{"name":"shot.png","mimeType":"image/png","sizeBytes":3}],"inline_files":[{"name":"shot.png","data_url":"data:image/png;base64,AAAA"}]},"messages":[],"version":9}`)
	}))
	taken, left, err := h.TakeBackQueuedTurnMessage("sess_shared", "q_1")
	if err != nil {
		t.Fatal(err)
	}
	if taken.ID != "q_1" || taken.Text != "look" || len(left) != 0 {
		t.Fatalf("taken = %+v, left = %+v, want the original message and an empty queue", taken, left)
	}
	if len(taken.ImageParts) != 1 || taken.ImageParts[0].Name != "shot.png" || taken.ImageParts[0].DataURL != "data:image/png;base64,AAAA" {
		t.Fatalf("taken message lost its attachment bytes: %+v", taken.ImageParts)
	}
}
