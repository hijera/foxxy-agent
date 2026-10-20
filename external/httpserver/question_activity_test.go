//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestQuestionActivityRestoresPendingPrompt(t *testing.T) {
	mgr, srv, _ := testHTTPServerPersist(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := mgr.HandleSessionNew(ctx, acp.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	params := acp.QuestionRequestParams{
		SessionID: sess.SessionID, RequestID: "q_recovery", ToolCallID: "call_question",
		Questions: []acp.QuestionPrompt{{Question: "What should be checked?", Options: []acp.QuestionOption{{Label: "Whole project"}}}},
	}
	buffer := &syncBuffer{}
	sender := NewSender(&config.Config{}, buffer, true, "agent")
	done := make(chan error, 1)
	go func() {
		answer, err := sender.RequestQuestion(ctx, params)
		if err == nil && (answer == nil || len(answer.Answers) != 1 || len(answer.Answers[0]) != 1 || answer.Answers[0][0] != "Whole project") {
			err = context.Canceled
		}
		done <- err
	}()
	for buffer.String() == "" {
		select {
		case <-ctx.Done():
			t.Fatal("question was not emitted")
		case <-time.After(time.Millisecond):
		}
	}
	server := httptest.NewServer(srv.Handler())
	defer server.Close()
	read := func() map[string]json.RawMessage {
		t.Helper()
		res, err := http.Get(server.URL + "/foxxycode/sessions/" + sess.SessionID + "/activity")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var body map[string]json.RawMessage
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	var pending acp.QuestionRequestParams
	body := read()
	if err := json.Unmarshal(body["pendingQuestion"], &pending); err != nil {
		t.Fatalf("activity did not carry the waiting question: %v; body=%s", err, body)
	}
	if pending.RequestID != params.RequestID || pending.ToolCallID != params.ToolCallID || pending.Questions[0].Options[0].Label != "Whole project" {
		t.Fatalf("wrong pending question: %+v", pending)
	}
	if string(body["turnActive"]) != "true" {
		t.Fatal("a pending question must keep the activity active")
	}
	if !CompleteQuestionAnswer(sess.SessionID, params.RequestID, &acp.QuestionResult{Answers: [][]string{{"Whole project"}}}) {
		t.Fatal("recovered request could not be answered")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, remains := read()["pendingQuestion"]; remains {
		t.Fatal("answered question remained in activity")
	}
}

func TestPendingQuestionClonesPayloadAndDisappearsOnCancel(t *testing.T) {
	params := acp.QuestionRequestParams{SessionID: t.Name(), RequestID: "q_cancel", Questions: []acp.QuestionPrompt{{Question: "Check?", Options: []acp.QuestionOption{{Label: "Original"}}}}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	buffer := &syncBuffer{}
	sender := NewSender(&config.Config{}, buffer, true, "agent")
	go func() { _, err := sender.RequestQuestion(ctx, params); done <- err }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.Now().Add(5 * time.Second)
	for buffer.String() == "" {
		if time.Now().After(deadline) {
			t.Fatal("question was not registered")
		}
		time.Sleep(time.Millisecond)
	}
	params.Questions[0].Options[0].Label = "Caller mutation"
	snapshot := pendingQuestion(params.SessionID)
	if snapshot.Questions[0].Options[0].Label != "Original" {
		t.Fatal("registration retained the caller's slice")
	}
	snapshot.Questions[0].Options[0].Label = "Client mutation"
	if pendingQuestion(params.SessionID).Questions[0].Options[0].Label != "Original" {
		t.Fatal("snapshot exposed the runtime's slice")
	}
	cancel()
	for pendingQuestion(params.SessionID) != nil {
		if time.Now().After(deadline) {
			t.Fatal("cancelled question remained live")
		}
		time.Sleep(time.Millisecond)
	}
}
