//go:build http

package httpserver

import (
	"sync"

	"github.com/hijera/foxxycode-agent/internal/acp"
)

type questionWaitKey struct {
	sessionID string
	requestID string
}

type questionWait struct {
	params acp.QuestionRequestParams
	reply  chan *acp.QuestionResult
	seq    uint64
}

var (
	questionWaitsMu sync.Mutex
	questionWaits   = make(map[questionWaitKey]questionWait)
	questionWaitSeq uint64
)

func registerQuestionWait(params acp.QuestionRequestParams) <-chan *acp.QuestionResult {
	questionWaitsMu.Lock()
	defer questionWaitsMu.Unlock()
	k := questionWaitKey{sessionID: params.SessionID, requestID: params.RequestID}
	ch := make(chan *acp.QuestionResult, 1)
	questionWaitSeq++
	questionWaits[k] = questionWait{params: cloneQuestionParams(params), reply: ch, seq: questionWaitSeq}
	return ch
}

func cloneQuestionParams(params acp.QuestionRequestParams) acp.QuestionRequestParams {
	params.Questions = append([]acp.QuestionPrompt(nil), params.Questions...)
	for i := range params.Questions {
		params.Questions[i].Options = append([]acp.QuestionOption(nil), params.Questions[i].Options...)
	}
	return params
}

// pendingQuestion returns the newest live gate of this session. It is runtime
// state, never reconstructed from unfinished tool records after a process exit.
// The scan walks every waiting gate on each activity poll: questions are asked
// sequentially per session and finish in seconds, so the map stays small enough
// that no per-session index is warranted.
func pendingQuestion(sessionID string) *acp.QuestionRequestParams {
	questionWaitsMu.Lock()
	defer questionWaitsMu.Unlock()
	var latest questionWait
	for key, wait := range questionWaits {
		if key.sessionID == sessionID && wait.seq > latest.seq {
			latest = wait
		}
	}
	if latest.seq == 0 {
		return nil
	}
	params := cloneQuestionParams(latest.params)
	return &params
}

func unregisterQuestionWait(sessionID, requestID string) {
	questionWaitsMu.Lock()
	defer questionWaitsMu.Unlock()
	delete(questionWaits, questionWaitKey{sessionID: sessionID, requestID: requestID})
}

// CompleteQuestionAnswer resolves a pending HTTP/streaming question. Returns false if nothing was waiting.
func CompleteQuestionAnswer(sessionID, requestID string, res *acp.QuestionResult) bool {
	questionWaitsMu.Lock()
	defer questionWaitsMu.Unlock()
	k := questionWaitKey{sessionID: sessionID, requestID: requestID}
	wait, ok := questionWaits[k]
	if !ok {
		return false
	}
	delete(questionWaits, k)
	wait.reply <- res
	return true
}
