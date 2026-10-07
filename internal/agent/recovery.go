package agent

import (
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

const recoveryInstruction = "Previous execution was interrupted. Continue from the saved conversation and completed tool results. Do not restart the task, repeat your introduction, or repeat a plan already stated. Check what actually completed; a tool call without a result has an unknown outcome. Verify its effects before retrying any action. If you were repeating the same steps, change your approach and perform the next concrete step, or explain the specific blocker. Respect the latest user request and do not resume work the user cancelled."
const loopCorrection = "Repeated actions produced no new information. You are stuck in a repetition loop. Do not repeat the same tool calls, re-read unchanged material, or announce again that you will begin. Use the results already available to take a different concrete next step. If you cannot make progress, explain the blocker and stop."

func continuationRequest(text string) bool {
	text = strings.Trim(strings.ToLower(strings.TrimSpace(text)), ".! ")
	switch text {
	case "continue", "resume", "go on", "продолжай", "продолжи", "продолжай дальше", "дальше", "продолжить":
		return true
	}
	// Match a continuation instruction at the start of a clause, rather than
	// arbitrary task words such as "Add a resume button" or "fix a stuck screen".
	for _, clause := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '.' || r == '!' || r == '?'
	}) {
		clause = strings.Join(strings.Fields(clause), " ")
		for _, preamble := range []string{"please ", "пожалуйста ", "после рестарта ", "после перезапуска ", "after a restart ", "after restarting "} {
			clause = strings.TrimPrefix(clause, preamble)
		}
		for _, command := range []string{"continue", "resume", "go on", "продолжай", "продолжи", "продолжить", "дальше", "you are stuck", "тебя зациклило", "ты зациклился"} {
			if clause == command || strings.HasPrefix(clause, command+" ") {
				return true
			}
		}
	}
	return false
}

// closeInterruptedToolCalls repairs only the model-facing history. Missing results
// describe uncertainty; they never claim success or trigger execution of old calls.
func closeInterruptedToolCalls(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		out = append(out, m)
		if m.Role != llm.RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		results := map[string]bool{}
		j := i + 1
		for ; j < len(msgs) && msgs[j].Role == llm.RoleTool; j++ {
			results[msgs[j].ToolCallID] = true
			out = append(out, msgs[j])
		}
		for _, tc := range m.ToolCalls {
			if !results[tc.ID] {
				out = append(out, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Content: fmt.Sprintf("Execution interrupted; no durable result is available for %s. Outcome unknown. Verify current state before retrying.", tc.Name)})
			}
		}
		i = j - 1
	}
	return out
}
