//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
)

// foxxycodeIdeToolResult answers POST /foxxycode/ide/tool-result: the IDE
// client's outcome for a tool_execute event it received on
// /foxxycode/ide/events. The body is {toolCallId, nonce, status, output?, error?}
// with status "handled" (the client executed the call; output or error is the
// tool result) or "not_handled" (declined; the agent runs the call itself).
func foxxycodeIdeToolResult(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ToolCallID string `json:"toolCallId"`
		Nonce      string `json:"nonce"`
		Status     string `json:"status"`
		Output     string `json:"output"`
		Error      string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON body"}}`, http.StatusBadRequest)
		return
	}
	status := strings.TrimSpace(body.Status)
	switch status {
	case acp.ToolExecHandled, acp.ToolExecNotHandled:
	case "":
		http.Error(w, `{"error":{"message":"status must be handled or not_handled"}}`, http.StatusBadRequest)
		return
	default:
		http.Error(w, `{"error":{"message":"status must be handled or not_handled"}}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.ToolCallID) == "" || strings.TrimSpace(body.Nonce) == "" {
		http.Error(w, `{"error":{"message":"toolCallId and nonce are required"}}`, http.StatusBadRequest)
		return
	}
	res := acp.ToolExecutionResult{Status: status, Output: body.Output, Err: body.Error}
	if !CompleteToolExecResult(body.ToolCallID, body.Nonce, res) {
		http.Error(w, `{"error":{"message":"no pending tool execution for this toolCallId, or the nonce does not match"}}`, http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
