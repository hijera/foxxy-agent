//go:build http

package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestCompactEndpointRefusesModelBeforeTurnAdmission(t *testing.T) {
	s := &compactHTTPFeatureState{root: t.TempDir()}
	if err := s.startServer(); err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if err := s.sessionWithExchanges(3); err != nil {
		t.Fatal(err)
	}
	_, finish, err := s.mgr.BeginSessionWork(context.Background(), s.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	for _, model := range []string{"nope", "fake/"} {
		if err := s.postCompactBody(fmt.Sprintf(`{"model":%q}`, model)); err != nil {
			t.Fatal(err)
		}
		if s.status != http.StatusBadRequest {
			t.Fatalf("model %q: status %d, body %v", model, s.status, s.body)
		}
	}
	for _, msg := range s.mgr.SessionByID(s.sessionID).GetMessages() {
		if msg.CompactionSummary {
			t.Fatal("invalid model changed the transcript")
		}
	}
}

func TestCompactEndpointRefusesReasoningBeforeTurnAdmission(t *testing.T) {
	s := &compactHTTPFeatureState{root: t.TempDir()}
	if err := s.startServer(); err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if err := s.sessionWithExchanges(3); err != nil {
		t.Fatal(err)
	}
	_, finish, err := s.mgr.BeginSessionWork(context.Background(), s.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	for _, body := range []string{`{"reasoning":"ultra"}`, `{"model":"qwen","reasoning":"ultra"}`} {
		if err := s.postCompactBody(body); err != nil {
			t.Fatal(err)
		}
		if s.status != http.StatusBadRequest {
			t.Fatalf("body %s: status %d, response %v", body, s.status, s.body)
		}
	}

	// A named summarizer makes the level invalid before the session lookup.
	s.sessionID = "sess_does_not_exist"
	if err := s.postCompactBody(`{"model":"qwen","reasoning":"ultra"}`); err != nil {
		t.Fatal(err)
	}
	if s.status != http.StatusBadRequest {
		t.Fatalf("unknown session: status %d, response %v", s.status, s.body)
	}
}
