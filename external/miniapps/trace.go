//go:build miniapps

package miniapps

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// TraceActionStatus is the status assigned to an observed tool invocation.
// Failed and denied invocations remain in authoring evidence, but are not
// compiled into a generated workflow.
type TraceActionStatus string

const (
	TraceActionSucceeded    TraceActionStatus = "succeeded"
	TraceActionFailed       TraceActionStatus = "failed"
	TraceActionDenied       TraceActionStatus = "denied"
	TraceActionMissing      TraceActionStatus = "missing_result"
	TraceActionCancelled    TraceActionStatus = "cancelled"
	TraceActionOrphanResult TraceActionStatus = "orphan_result"
)

// TraceMessage is the sanitized, persisted message representation used by the
// distiller. It deliberately excludes provider reasoning and image bytes.
type TraceMessage struct {
	Index      int             `json:"index"`
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []TraceToolCall `json:"tool_calls,omitempty"`
	CreatedAt  string          `json:"created_at,omitempty"`
}

// TraceToolCall is a sanitized assistant tool-call request.
type TraceToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// TraceArtifact identifies an artifact produced by a call. Paths and hashes
// are metadata only; artifact contents never enter the portable MiniApp.
type TraceArtifact struct {
	Path      string `json:"path,omitempty"`
	Kind      string `json:"kind,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// TraceCallEvidence is an optional per-call DTO loaded from the session's
// private tool-call evidence store. It supplements persisted llm.Messages.
type TraceCallEvidence struct {
	ID         string            `json:"id,omitempty"`
	CallID     string            `json:"call_id,omitempty"`
	ToolCallID string            `json:"toolCallId,omitempty"`
	Name       string            `json:"name,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Status     TraceActionStatus `json:"status,omitempty"`
	Arguments  string            `json:"arguments,omitempty"`
	Result     string            `json:"result,omitempty"`
	Error      string            `json:"error,omitempty"`
	Permission string            `json:"permission,omitempty"`
	StartedAt  string            `json:"started_at,omitempty"`
	FinishedAt string            `json:"finished_at,omitempty"`
	DurationMS int64             `json:"duration_ms,omitempty"`
	Artifacts  []TraceArtifact   `json:"artifacts,omitempty"`
}

// UnmarshalJSON accepts both the snake-case authoring DTO and the camel-case
// keys written by internal/session.ToolCallMeta. Arguments may be a JSON
// object on disk or a pre-encoded string supplied by an adapter.
func (e *TraceCallEvidence) UnmarshalJSON(data []byte) error {
	type wire struct {
		ID              string            `json:"id"`
		CallID          string            `json:"call_id"`
		CallIDCamel     string            `json:"callId"`
		ToolCallID      string            `json:"toolCallId"`
		Name            string            `json:"name"`
		Kind            string            `json:"kind"`
		Status          TraceActionStatus `json:"status"`
		Arguments       json.RawMessage   `json:"arguments"`
		ArgumentsInput  json.RawMessage   `json:"input"`
		Error           string            `json:"error"`
		Result          string            `json:"result"`
		Permission      string            `json:"permission"`
		StartedAt       string            `json:"started_at"`
		StartedAtCamel  string            `json:"startedAt"`
		FinishedAt      string            `json:"finished_at"`
		FinishedAtCamel string            `json:"finishedAt"`
		DurationMS      int64             `json:"duration_ms"`
		Artifacts       []TraceArtifact   `json:"artifacts"`
	}
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = TraceCallEvidence{
		ID: decoded.ID, CallID: decoded.CallID, ToolCallID: decoded.ToolCallID,
		Name: decoded.Name, Kind: decoded.Kind, Status: decoded.Status,
		Error: decoded.Error, Result: decoded.Result, Permission: decoded.Permission,
		StartedAt: decoded.StartedAt, FinishedAt: decoded.FinishedAt,
		DurationMS: decoded.DurationMS, Artifacts: decoded.Artifacts,
	}
	if e.CallID == "" {
		e.CallID = decoded.CallIDCamel
	}
	if e.StartedAt == "" {
		e.StartedAt = decoded.StartedAtCamel
	}
	if e.FinishedAt == "" {
		e.FinishedAt = decoded.FinishedAtCamel
	}
	args := decoded.Arguments
	if len(args) == 0 {
		args = decoded.ArgumentsInput
	}
	if len(args) > 0 && string(args) != "null" {
		var text string
		if json.Unmarshal(args, &text) == nil {
			e.Arguments = text
		} else {
			e.Arguments = string(args)
		}
	}
	return nil
}

// TraceEvidence is retained as a descriptive alias for callers that obtain
// evidence from a disk adapter.
type TraceEvidence = TraceCallEvidence

// TraceAction is one observed tool invocation paired with its result when one
// was persisted. A missing or failed result is intentionally represented.
type TraceAction struct {
	Index              int               `json:"index"`
	SourceMessageIndex int               `json:"source_message_index,omitempty"`
	SourceTurn         int               `json:"source_turn,omitempty"`
	CallID             string            `json:"call_id,omitempty"`
	Name               string            `json:"name"`
	Kind               string            `json:"kind,omitempty"`
	Status             TraceActionStatus `json:"status"`
	Arguments          string            `json:"arguments,omitempty"`
	Result             string            `json:"result,omitempty"`
	Error              string            `json:"error,omitempty"`
	ResultFound        bool              `json:"result_found"`
	MissingResult      bool              `json:"missing_result,omitempty"`
	Denied             bool              `json:"denied,omitempty"`
	Failed             bool              `json:"failed,omitempty"`
	Orphan             bool              `json:"orphan,omitempty"`
	Permission         string            `json:"permission,omitempty"`
	StartedAt          string            `json:"started_at,omitempty"`
	FinishedAt         string            `json:"finished_at,omitempty"`
	DurationMS         int64             `json:"duration_ms,omitempty"`
	Artifacts          []TraceArtifact   `json:"artifacts,omitempty"`
}

// NormalizedTrace is the private, sanitized source trace used by scenario
// selection and workflow synthesis.
type NormalizedTrace struct {
	SessionID               string         `json:"session_id,omitempty"`
	Messages                []TraceMessage `json:"messages,omitempty"`
	Actions                 []TraceAction  `json:"actions,omitempty"`
	ToolCallCount           int            `json:"tool_call_count"`
	SuccessfulToolCallCount int            `json:"successful_tool_call_count"`
	HasAssistantResult      bool           `json:"has_assistant_result"`
	LastAssistantResult     string         `json:"last_assistant_result,omitempty"`
}

// TraceInput is the source data accepted by normalization. Evidence is
// optional because older sessions only persist llm.Messages.
type TraceInput struct {
	SessionID  string
	Messages   []llm.Message
	Evidence   []TraceCallEvidence
	TurnActive bool
}

const (
	TraceEligibilityEligible     = "eligible"
	TraceEligibilityNotSuitable  = "not_suitable"
	TraceEligibilityNeedsClarify = "needs_clarification"
	TraceEligibilityRunning      = "running"
)

// TraceEligibility is the deterministic gate before scenario authoring.
type TraceEligibility struct {
	Status              string `json:"status"`
	Eligible            bool   `json:"eligible"`
	Reason              string `json:"reason,omitempty"`
	ToolCalls           int    `json:"tool_calls"`
	SuccessfulToolCalls int    `json:"successful_tool_calls"`
}

// TraceScenarioCandidate is one independently identifiable task in a trace.
type TraceScenarioCandidate struct {
	ID                     string   `json:"id"`
	Task                   string   `json:"task"`
	AcceptedOutcome        string   `json:"accepted_outcome"`
	ActionIndexes          []int    `json:"action_indexes,omitempty"`
	EvidenceMessageIndexes []int    `json:"evidence_message_indexes,omitempty"`
	Boundaries             []string `json:"boundaries,omitempty"`
	Confidence             float64  `json:"confidence"`
}

// TraceScenarioCorrection contains operator-edited scenario fields.
type TraceScenarioCorrection struct {
	CandidateID     string   `json:"candidate_id,omitempty"`
	Task            string   `json:"task,omitempty"`
	AcceptedOutcome string   `json:"accepted_outcome,omitempty"`
	ActionIndexes   []int    `json:"action_indexes,omitempty"`
	Boundaries      []string `json:"boundaries,omitempty"`
}

// TraceScenarioSelection identifies a candidate and optionally supplies a
// correction made during the confirmation step.
type TraceScenarioSelection struct {
	CandidateID string                   `json:"candidate_id,omitempty"`
	Correction  *TraceScenarioCorrection `json:"correction,omitempty"`
}

// TraceConfirmedScenario is the scenario accepted by the author.
type TraceConfirmedScenario struct {
	CandidateID     string   `json:"candidate_id"`
	Task            string   `json:"task"`
	AcceptedOutcome string   `json:"accepted_outcome"`
	ActionIndexes   []int    `json:"action_indexes"`
	Boundaries      []string `json:"boundaries,omitempty"`
}

// TraceInputClass describes how an observed value should be represented in a
// generated MiniApp.
type TraceInputClass string

const (
	TraceInputFixed          TraceInputClass = "fixed"
	TraceInputOperator       TraceInputClass = "operator"
	TraceInputSecret         TraceInputClass = "secret"
	TraceInputPriorStep      TraceInputClass = "prior_step"
	TraceInputEnvironment    TraceInputClass = "environment"
	TraceInputSourceSpecific TraceInputClass = "source_specific"
)

// TraceValueOccurrence points to one JSON value in one observed action.
type TraceValueOccurrence struct {
	ActionIndex int    `json:"action_index"`
	JSONPath    string `json:"json_path"`
	Key         string `json:"key,omitempty"`
}

// TraceInputSpec is the private classification result used to generate Input
// declarations and argument references.
type TraceInputSpec struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	Title            string                 `json:"title"`
	Description      string                 `json:"description,omitempty"`
	Class            TraceInputClass        `json:"class"`
	ObservedValue    any                    `json:"observed_value,omitempty"`
	Default          any                    `json:"default,omitempty"`
	Required         bool                   `json:"required"`
	Occurrences      []TraceValueOccurrence `json:"occurrences,omitempty"`
	PriorActionIndex *int                   `json:"prior_action_index,omitempty"`
}

// ScenarioConfirmationError is returned when distillation has enough evidence
// to offer candidates but no candidate has been confirmed yet.
type ScenarioConfirmationError struct {
	Candidates []TraceScenarioCandidate
}

func (e *ScenarioConfirmationError) Error() string {
	return fmt.Sprintf("scenario confirmation required (%d candidate(s))", len(e.Candidates))
}

var errTraceNoScenario = errors.New("scenario confirmation required")

// NormalizeSessionTrace builds a deterministic sanitized trace from persisted
// messages and optional disk evidence. Pairing is by tool-call ID, never by
// position, because providers may reorder or omit result messages.
func NormalizeSessionTrace(sessionID string, messages []llm.Message, evidence []TraceCallEvidence) (NormalizedTrace, error) {
	trace := NormalizedTrace{SessionID: sessionID}
	trace.Messages = make([]TraceMessage, 0, len(messages))
	secretLiterals := collectTraceSecretLiterals(messages, evidence)

	type resultMessage struct {
		content   string
		createdAt string
	}
	results := make(map[string][]resultMessage)
	resultOrder := make([]string, 0)
	for index, message := range messages {
		normalized := TraceMessage{
			Index: index, Role: string(message.Role),
			Content: sanitizeTraceText(message.Content), ToolCallID: message.ToolCallID,
			CreatedAt: message.CreatedAt,
		}
		for _, call := range message.ToolCalls {
			normalized.ToolCalls = append(normalized.ToolCalls, TraceToolCall{
				ID: call.ID, Name: call.Name, Arguments: sanitizeJSONText(call.InputJSON),
			})
		}
		trace.Messages = append(trace.Messages, normalized)
		if message.Role == llm.RoleTool && strings.TrimSpace(message.ToolCallID) != "" {
			if _, exists := results[message.ToolCallID]; !exists {
				resultOrder = append(resultOrder, message.ToolCallID)
			}
			results[message.ToolCallID] = append(results[message.ToolCallID], resultMessage{
				content: sanitizeTraceText(message.Content), createdAt: message.CreatedAt,
			})
		}
		if message.Role == llm.RoleAssistant && strings.TrimSpace(sanitizeTraceText(message.Content)) != "" {
			trace.HasAssistantResult = true
			trace.LastAssistantResult = sanitizeTraceText(message.Content)
		}
	}

	trace.Actions = make([]TraceAction, 0)
	usedEvidence := make([]bool, len(evidence))
	for messageIndex, message := range messages {
		if message.Role != llm.RoleAssistant {
			continue
		}
		for _, call := range message.ToolCalls {
			action := TraceAction{
				Index: len(trace.Actions), SourceMessageIndex: messageIndex, SourceTurn: messageIndex,
				CallID: call.ID, Name: call.Name, Arguments: sanitizeJSONText(call.InputJSON),
				Status: TraceActionMissing, MissingResult: true,
			}
			if queue := results[call.ID]; len(queue) > 0 {
				result := queue[0]
				results[call.ID] = queue[1:]
				action.Result = result.content
				action.ResultFound = true
				action.MissingResult = false
				action.Status = inferTraceActionStatus("", result.content)
				action.FinishedAt = result.createdAt
			}
			mergeEvidenceIntoAction(&action, evidence, usedEvidence)
			finalizeTraceAction(&action)
			trace.Actions = append(trace.Actions, action)
		}
	}

	// Keep disk evidence that did not have a persisted assistant request. This
	// handles older sessions where tool calls were evicted from the transcript.
	for index, item := range evidence {
		if usedEvidence[index] {
			continue
		}
		id := evidenceID(item)
		if id == "" {
			continue
		}
		action := TraceAction{
			Index: len(trace.Actions), CallID: id, Name: item.Name, Kind: item.Kind,
			Arguments: sanitizeJSONText(item.Arguments), Result: sanitizeTraceText(item.Result),
			Error: sanitizeTraceText(item.Error), Status: item.Status, Orphan: true,
			Permission: sanitizeTraceText(item.Permission), StartedAt: item.StartedAt,
			FinishedAt: item.FinishedAt, DurationMS: item.DurationMS,
			Artifacts: sanitizeArtifacts(item.Artifacts),
		}
		if action.Status == "" {
			if action.Result != "" || action.Error != "" {
				action.Status = inferTraceActionStatus(action.Error, action.Result)
			} else {
				action.Status = TraceActionMissing
			}
		}
		finalizeTraceAction(&action)
		trace.Actions = append(trace.Actions, action)
	}

	// Also retain orphan role=tool rows. They are useful evidence and should not
	// be mistaken for a successful, replayable call.
	for _, id := range resultOrder {
		queue := results[id]
		for _, result := range queue {
			action := TraceAction{
				Index: len(trace.Actions), CallID: id, Status: inferTraceActionStatus("", result.content),
				Result: result.content, ResultFound: true, Orphan: true,
			}
			if action.Status == TraceActionSucceeded {
				action.Status = TraceActionOrphanResult
			}
			finalizeTraceAction(&action)
			trace.Actions = append(trace.Actions, action)
		}
	}

	trace.ToolCallCount = len(trace.Actions)
	for _, action := range trace.Actions {
		if action.Status == TraceActionSucceeded && !action.Orphan {
			trace.SuccessfulToolCallCount++
		}
	}
	redactNormalizedTraceSecrets(&trace, secretLiterals)
	return trace, nil
}

// ExtractNormalizedTrace is the struct-oriented normalization entry point.
func ExtractNormalizedTrace(input TraceInput) (NormalizedTrace, error) {
	return NormalizeSessionTrace(input.SessionID, input.Messages, input.Evidence)
}

func mergeEvidenceIntoAction(action *TraceAction, evidence []TraceCallEvidence, used []bool) {
	for index, item := range evidence {
		if used[index] || evidenceID(item) == "" || evidenceID(item) != action.CallID {
			continue
		}
		used[index] = true
		if action.Name == "" {
			action.Name = item.Name
		}
		if action.Kind == "" {
			action.Kind = item.Kind
		}
		if action.Arguments == "" {
			action.Arguments = sanitizeJSONText(item.Arguments)
		}
		if action.Result == "" {
			action.Result = sanitizeTraceText(item.Result)
		}
		if action.Error == "" {
			action.Error = sanitizeTraceText(item.Error)
		}
		if action.Permission == "" {
			action.Permission = sanitizeTraceText(item.Permission)
		}
		if action.StartedAt == "" {
			action.StartedAt = item.StartedAt
		}
		if action.FinishedAt == "" {
			action.FinishedAt = item.FinishedAt
		}
		if action.DurationMS == 0 {
			action.DurationMS = item.DurationMS
		}
		if len(action.Artifacts) == 0 {
			action.Artifacts = sanitizeArtifacts(item.Artifacts)
		}
		if item.Status != "" {
			action.Status = item.Status
		}
		if action.Result != "" {
			action.ResultFound = true
			action.MissingResult = false
		}
		return
	}
}

func evidenceID(item TraceCallEvidence) string {
	if strings.TrimSpace(item.ID) != "" {
		return strings.TrimSpace(item.ID)
	}
	if strings.TrimSpace(item.ToolCallID) != "" {
		return strings.TrimSpace(item.ToolCallID)
	}
	return strings.TrimSpace(item.CallID)
}

func finalizeTraceAction(action *TraceAction) {
	action.Arguments = sanitizeJSONText(action.Arguments)
	action.Result = sanitizeTraceText(action.Result)
	action.Error = sanitizeTraceText(action.Error)
	action.Permission = sanitizeTraceText(action.Permission)
	action.Artifacts = sanitizeArtifacts(action.Artifacts)
	if action.Status == "" {
		if action.ResultFound || action.Result != "" || action.Error != "" {
			action.Status = inferTraceActionStatus(action.Error, action.Result)
		} else {
			action.Status = TraceActionMissing
		}
	}
	action.Status = normalizeTraceActionStatus(action.Status)
	action.Denied = action.Status == TraceActionDenied
	action.Failed = action.Status == TraceActionFailed || action.Status == TraceActionDenied || action.Status == TraceActionCancelled
	action.MissingResult = action.Status == TraceActionMissing
	if action.Result != "" {
		action.ResultFound = true
	}
	if action.StartedAt != "" && action.FinishedAt != "" {
		start, startErr := time.Parse(time.RFC3339Nano, action.StartedAt)
		finish, finishErr := time.Parse(time.RFC3339Nano, action.FinishedAt)
		if startErr == nil && finishErr == nil && !finish.Before(start) {
			action.DurationMS = finish.Sub(start).Milliseconds()
		}
	}
}

func normalizeTraceActionStatus(status TraceActionStatus) TraceActionStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case "ok", "success", "succeeded", "completed", "complete":
		return TraceActionSucceeded
	case "denied", "forbidden", "permission_denied", "permission-denied":
		return TraceActionDenied
	case "cancelled", "canceled", "interrupted":
		return TraceActionCancelled
	case "failed", "failure", "error":
		return TraceActionFailed
	case "orphan_result":
		return TraceActionOrphanResult
	case "missing", "missing_result", "pending", "":
		return TraceActionMissing
	default:
		return status
	}
}

func inferTraceActionStatus(errText, result string) TraceActionStatus {
	combined := strings.ToLower(strings.TrimSpace(errText + "\n" + result))
	if combined == "" {
		return TraceActionMissing
	}
	for _, marker := range []string{"permission denied", "access denied", "forbidden", "not allowed", "rejected"} {
		if strings.Contains(combined, marker) {
			return TraceActionDenied
		}
	}
	for _, marker := range []string{"error:", "failed", "failure", "exception", "cancelled", "canceled", "interrupted"} {
		if strings.Contains(combined, marker) {
			if strings.Contains(combined, "cancel") || strings.Contains(combined, "interrupt") {
				return TraceActionCancelled
			}
			return TraceActionFailed
		}
	}
	return TraceActionSucceeded
}

// AssessTraceEligibility applies the non-model eligibility gate.
func AssessTraceEligibility(trace NormalizedTrace, turnActive bool) TraceEligibility {
	result := TraceEligibility{ToolCalls: trace.ToolCallCount, SuccessfulToolCalls: trace.SuccessfulToolCallCount}
	if turnActive {
		result.Status = TraceEligibilityRunning
		result.Reason = "the session has an active turn"
		return result
	}
	user := false
	for _, message := range trace.Messages {
		if message.Role == string(llm.RoleUser) && strings.TrimSpace(message.Content) != "" {
			user = true
			break
		}
	}
	if !user {
		result.Status = TraceEligibilityNotSuitable
		result.Reason = "the session has no completed user task"
		return result
	}
	if !trace.HasAssistantResult {
		result.Status = TraceEligibilityNeedsClarify
		result.Reason = "the session has no assistant outcome"
		return result
	}
	if trace.SuccessfulToolCallCount == 0 {
		result.Status = TraceEligibilityNotSuitable
		result.Reason = "conversation-only sessions are not yet distillable"
		return result
	}
	result.Status = TraceEligibilityEligible
	result.Eligible = true
	return result
}

// GenerateScenarioCandidates segments a trace at user turns. Failed attempts
// remain in the candidate evidence, while successful actions drive synthesis.
func GenerateScenarioCandidates(trace NormalizedTrace) []TraceScenarioCandidate {
	userMessages := make([]int, 0)
	for _, message := range trace.Messages {
		if message.Role == string(llm.RoleUser) && strings.TrimSpace(message.Content) != "" {
			userMessages = append(userMessages, message.Index)
		}
	}
	if len(userMessages) == 0 {
		return nil
	}
	candidates := make([]TraceScenarioCandidate, 0, len(userMessages))
	for segment, start := range userMessages {
		end := len(trace.Messages)
		if segment+1 < len(userMessages) {
			end = userMessages[segment+1]
		}
		task := ""
		outcome := ""
		messageIndexes := make([]int, 0)
		for _, message := range trace.Messages {
			if message.Index < start || message.Index >= end {
				continue
			}
			messageIndexes = append(messageIndexes, message.Index)
			if message.Index == start {
				task = strings.TrimSpace(message.Content)
			}
			if message.Role == string(llm.RoleAssistant) && strings.TrimSpace(message.Content) != "" {
				outcome = strings.TrimSpace(message.Content)
			}
		}
		actionIndexes := make([]int, 0)
		failed := 0
		succeeded := 0
		for index, action := range trace.Actions {
			if action.Orphan || action.SourceMessageIndex < start || action.SourceMessageIndex >= end {
				continue
			}
			actionIndexes = append(actionIndexes, index)
			switch action.Status {
			case TraceActionSucceeded:
				succeeded++
			case TraceActionFailed, TraceActionDenied, TraceActionCancelled, TraceActionMissing:
				failed++
			}
		}
		if succeeded == 0 {
			continue
		}
		if outcome == "" {
			outcome = trace.LastAssistantResult
		}
		confidence := 0.55 + 0.15*float64(succeeded)
		if failed > 0 {
			confidence -= 0.1 * float64(failed)
		}
		if confidence > 0.99 {
			confidence = 0.99
		}
		if confidence < 0.1 {
			confidence = 0.1
		}
		candidates = append(candidates, TraceScenarioCandidate{
			ID: fmt.Sprintf("scenario-%d", len(candidates)+1), Task: task,
			AcceptedOutcome: outcome, ActionIndexes: actionIndexes,
			EvidenceMessageIndexes: messageIndexes,
			Boundaries:             []string{fmt.Sprintf("messages[%d:%d]", start, end)}, Confidence: confidence,
		})
	}
	return candidates
}

// ConfirmScenario resolves a candidate or applies an explicit correction.
func ConfirmScenario(candidates []TraceScenarioCandidate, selection TraceScenarioSelection) (TraceConfirmedScenario, error) {
	if len(candidates) == 0 {
		return TraceConfirmedScenario{}, errTraceNoScenario
	}
	selected := -1
	for index, candidate := range candidates {
		if selection.CandidateID != "" && candidate.ID == selection.CandidateID {
			selected = index
			break
		}
	}
	if selected < 0 && selection.CandidateID == "" && len(candidates) == 1 {
		selected = 0
	}
	if selected < 0 {
		return TraceConfirmedScenario{}, fmt.Errorf("unknown scenario candidate %q", selection.CandidateID)
	}
	candidate := candidates[selected]
	confirmed := TraceConfirmedScenario{
		CandidateID: candidate.ID, Task: strings.TrimSpace(candidate.Task),
		AcceptedOutcome: strings.TrimSpace(candidate.AcceptedOutcome),
		ActionIndexes:   append([]int(nil), candidate.ActionIndexes...),
		Boundaries:      append([]string(nil), candidate.Boundaries...),
	}
	if selection.Correction != nil {
		correction := selection.Correction
		if strings.TrimSpace(correction.CandidateID) != "" && correction.CandidateID != candidate.ID {
			return TraceConfirmedScenario{}, fmt.Errorf("scenario correction references %q, selected %q", correction.CandidateID, candidate.ID)
		}
		if strings.TrimSpace(correction.Task) != "" {
			confirmed.Task = strings.TrimSpace(correction.Task)
		}
		if strings.TrimSpace(correction.AcceptedOutcome) != "" {
			confirmed.AcceptedOutcome = strings.TrimSpace(correction.AcceptedOutcome)
		}
		if correction.ActionIndexes != nil {
			confirmed.ActionIndexes = append([]int(nil), correction.ActionIndexes...)
		}
		if correction.Boundaries != nil {
			confirmed.Boundaries = append([]string(nil), correction.Boundaries...)
		}
	}
	if confirmed.Task == "" || confirmed.AcceptedOutcome == "" {
		return TraceConfirmedScenario{}, fmt.Errorf("scenario candidate %q needs a task and accepted outcome", candidate.ID)
	}
	valid := make(map[int]bool, len(candidate.ActionIndexes))
	for _, index := range candidate.ActionIndexes {
		valid[index] = true
	}
	for _, index := range confirmed.ActionIndexes {
		if !valid[index] {
			return TraceConfirmedScenario{}, fmt.Errorf("scenario action index %d is outside candidate %q", index, candidate.ID)
		}
	}
	return confirmed, nil
}

// ConfirmTraceScenario is a descriptive alias for HTTP/service adapters.
func ConfirmTraceScenario(candidates []TraceScenarioCandidate, selection TraceScenarioSelection) (TraceConfirmedScenario, error) {
	return ConfirmScenario(candidates, selection)
}

// ClassifyTraceInputs identifies values that can become explicit MiniApp
// inputs. Results are ordered by action and JSON path for reproducibility.
func ClassifyTraceInputs(trace NormalizedTrace, scenario TraceConfirmedScenario) []TraceInputSpec {
	selected := make(map[int]bool, len(scenario.ActionIndexes))
	for _, index := range scenario.ActionIndexes {
		selected[index] = true
	}
	userText := ""
	for _, message := range trace.Messages {
		if message.Role == string(llm.RoleUser) {
			userText += " " + strings.ToLower(message.Content)
		}
	}
	type aggregate struct {
		TraceInputSpec
		firstAction int
		firstPath   string
	}
	aggregates := make(map[string]*aggregate)
	order := make([]string, 0)
	for actionIndex, action := range trace.Actions {
		if !selected[actionIndex] || action.Status != TraceActionSucceeded {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(action.Arguments), &value); err != nil {
			continue
		}
		walkTraceValues(value, "$", "", func(path, key string, value any) {
			if key == "" {
				return
			}
			class := classifyTraceValue(key, value, userText)
			var priorActionIndex *int
			if class != TraceInputSecret {
				if prior, matched := unambiguousPriorTraceResult(trace, selected, actionIndex, value); matched {
					class = TraceInputPriorStep
					priorActionIndex = &prior
				}
			}
			canonical := traceInputAggregateKey(path, key, class, value, priorActionIndex)
			entry := aggregates[canonical]
			if entry == nil {
				id := traceInputID(key, class, len(order)+1)
				entry = &aggregate{TraceInputSpec: TraceInputSpec{
					ID: id, Type: traceInputType(value), Title: humanizeTraceKey(key),
					Description: traceInputDescription(class), Class: class,
					ObservedValue: sanitizeTraceValue(value), Required: class == TraceInputOperator || class == TraceInputSecret,
					PriorActionIndex: priorActionIndex,
				}, firstAction: actionIndex, firstPath: path}
				aggregates[canonical] = entry
				order = append(order, canonical)
			}
			entry.Occurrences = append(entry.Occurrences, TraceValueOccurrence{ActionIndex: actionIndex, JSONPath: path, Key: key})
		})
	}
	result := make([]TraceInputSpec, 0, len(order))
	for _, key := range order {
		result = append(result, aggregates[key].TraceInputSpec)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if len(left.Occurrences) == 0 || len(right.Occurrences) == 0 {
			return left.ID < right.ID
		}
		if left.Occurrences[0].ActionIndex != right.Occurrences[0].ActionIndex {
			return left.Occurrences[0].ActionIndex < right.Occurrences[0].ActionIndex
		}
		return left.Occurrences[0].JSONPath < right.Occurrences[0].JSONPath
	})
	return result
}

func traceInputAggregateKey(path, key string, class TraceInputClass, value any, priorActionIndex *int) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded = []byte(fmt.Sprintf("%T:%v", value, value))
	}
	prior := ""
	if priorActionIndex != nil {
		prior = strconv.Itoa(*priorActionIndex)
	}
	return path + "\x00" + key + "\x00" + string(class) + "\x00" + string(encoded) + "\x00" + prior
}

func unambiguousPriorTraceResult(trace NormalizedTrace, selected map[int]bool, currentAction int, value any) (int, bool) {
	matched := -1
	for actionIndex := 0; actionIndex < currentAction && actionIndex < len(trace.Actions); actionIndex++ {
		action := trace.Actions[actionIndex]
		if !selected[actionIndex] || action.Orphan || action.Status != TraceActionSucceeded || (!action.ResultFound && action.Result == "") || !traceResultExactlyMatches(action.Result, value) {
			continue
		}
		if matched >= 0 {
			return 0, false
		}
		matched = actionIndex
	}
	return matched, matched >= 0
}

func traceResultExactlyMatches(result string, value any) bool {
	if text, ok := value.(string); ok && text == result {
		return true
	}
	var decoded any
	return json.Unmarshal([]byte(result), &decoded) == nil && reflect.DeepEqual(decoded, value)
}

func walkTraceValues(value any, path, key string, visit func(path, key string, value any)) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, childKey := range keys {
			childPath := path + "." + childKey
			child := typed[childKey]
			visit(childPath, childKey, child)
			walkTraceValues(child, childPath, childKey, visit)
		}
	case []any:
		for index, child := range typed {
			childPath := path + "[" + strconv.Itoa(index) + "]"
			walkTraceValues(child, childPath, key, visit)
		}
	}
}

func classifyTraceValue(key string, value any, userText string) TraceInputClass {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if isSecretKey(lowerKey) {
		return TraceInputSecret
	}
	if strings.Contains(lowerKey, "$ref") || strings.Contains(lowerKey, "prior") {
		return TraceInputPriorStep
	}
	if strings.Contains(lowerKey, "source") || strings.Contains(lowerKey, "fixture") || strings.Contains(lowerKey, "session") {
		return TraceInputSourceSpecific
	}
	if isEnvironmentKey(lowerKey) {
		return TraceInputEnvironment
	}
	if stringValue, ok := value.(string); ok {
		trimmed := strings.TrimSpace(stringValue)
		if strings.Contains(trimmed, "{{") || strings.HasPrefix(trimmed, "$ref:") || strings.HasPrefix(trimmed, "steps.") {
			return TraceInputPriorStep
		}
		if trimmed != "" && strings.Contains(userText, strings.ToLower(trimmed)) {
			return TraceInputOperator
		}
	}
	if isLikelyOperatorKey(lowerKey) {
		return TraceInputOperator
	}
	return TraceInputFixed
}

func isSecretKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	switch normalized {
	case "auth", "authorization", "credential", "credentials":
		return true
	}
	for _, suffix := range []string{"_auth", "_authorization", "_credential", "_credentials"} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	for _, marker := range []string{"api_key", "apikey", "api_token", "apitoken", "token", "password", "passwd", "secret", "private_key"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isEnvironmentKey(key string) bool {
	for _, marker := range []string{"cwd", "working_dir", "working_directory", "workspace", "home", "environment", "env", "session_dir", "root_dir"} {
		if key == marker || strings.HasSuffix(key, "_"+marker) {
			return true
		}
	}
	return false
}

func isLikelyOperatorKey(key string) bool {
	for _, marker := range []string{"query", "request", "prompt", "input", "path", "file", "content", "message", "name", "channel", "value"} {
		if key == marker || strings.HasSuffix(key, "_"+marker) {
			return true
		}
	}
	return false
}

func traceInputID(key string, class TraceInputClass, ordinal int) string {
	base := strings.ToLower(strings.TrimSpace(key))
	base = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "value"
	}
	prefix := "input"
	if class == TraceInputSecret {
		prefix = "secret"
	}
	if class == TraceInputEnvironment {
		prefix = "environment"
	}
	return fmt.Sprintf("%s_%s_%d", prefix, base, ordinal)
}

func traceInputType(value any) string {
	switch value.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case []any, map[string]any:
		return "json"
	default:
		return "text"
	}
}

func humanizeTraceKey(key string) string {
	key = strings.ReplaceAll(strings.ReplaceAll(key, "_", " "), "-", " ")
	key = strings.TrimSpace(key)
	if key == "" {
		return "Input"
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

func traceInputDescription(class TraceInputClass) string {
	switch class {
	case TraceInputSecret:
		return "Resolved at run time from a secret handle."
	case TraceInputEnvironment:
		return "Resolved from the operator's execution environment."
	case TraceInputPriorStep:
		return "Resolved from a previous workflow step."
	case TraceInputOperator:
		return "Provided by the operator when the MiniApp runs."
	case TraceInputSourceSpecific:
		return "Retained only in private authoring evidence."
	default:
		return "Observed constant from the source session."
	}
}

func sanitizeJSONText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) == nil {
		redacted := sanitizeTraceValue(decoded)
		encoded, err := json.Marshal(redacted)
		if err == nil {
			return string(encoded)
		}
	}
	return sanitizeTraceText(value)
}

func sanitizeTraceValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if isSecretKey(strings.ToLower(key)) {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = sanitizeTraceValue(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = sanitizeTraceValue(child)
		}
		return result
	case string:
		return sanitizeTraceText(typed)
	default:
		return value
	}
}

var (
	tracePrivateKeyRE   = regexp.MustCompile(`(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
	traceBearerRE       = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/-]+=*`)
	traceAPIKeyRE       = regexp.MustCompile(`(?i)\bsk-[a-z0-9_-]{8,}\b`)
	traceSecretAssignRE = regexp.MustCompile(`(?i)(api[_-]?key|api[_-]?token|password|passwd|secret|token|credential|authorization)\s*([:=])\s*("[^"]*"|'[^']*'|[^\s,;}]+)`)
	traceContextBlockRE = regexp.MustCompile(`(?is)<(?:foxxycode_(?:ide_context|terminal_context|terminal_output|session_assets|memory_context|injected_context)|environment_context|intellij_idea_project_context)\b[^>]*>.*?</(?:foxxycode_(?:ide_context|terminal_context|terminal_output|session_assets|memory_context|injected_context)|environment_context|intellij_idea_project_context)\s*>`)
	traceContextOpenRE  = regexp.MustCompile(`(?is)<(?:foxxycode_(?:ide_context|terminal_context|terminal_output|session_assets|memory_context|injected_context)|environment_context|intellij_idea_project_context)\b[^>]*>.*$`)
)

func sanitizeTraceText(value string) string {
	value = traceContextBlockRE.ReplaceAllString(value, "")
	value = traceContextOpenRE.ReplaceAllString(value, "")
	value = tracePrivateKeyRE.ReplaceAllString(value, "[REDACTED]")
	value = traceBearerRE.ReplaceAllString(value, "Bearer [REDACTED]")
	value = traceAPIKeyRE.ReplaceAllString(value, "[REDACTED]")
	value = traceSecretAssignRE.ReplaceAllString(value, "$1$2[REDACTED]")
	return value
}

func collectTraceSecretLiterals(messages []llm.Message, evidence []TraceCallEvidence) []string {
	unique := make(map[string]bool)
	collect := func(arguments string) {
		var decoded any
		if json.Unmarshal([]byte(arguments), &decoded) == nil {
			collectStructuredSecretLiterals(decoded, "", unique)
		}
	}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			collect(call.InputJSON)
		}
	}
	for _, item := range evidence {
		collect(item.Arguments)
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if len(result[left]) == len(result[right]) {
			return result[left] < result[right]
		}
		return len(result[left]) > len(result[right])
	})
	return result
}

func collectStructuredSecretLiterals(value any, key string, result map[string]bool) {
	if key != "" && isSecretKey(key) {
		collectTraceLeafStrings(value, result)
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			collectStructuredSecretLiterals(child, childKey, result)
		}
	case []any:
		for _, child := range typed {
			collectStructuredSecretLiterals(child, key, result)
		}
	}
}

func collectTraceLeafStrings(value any, result map[string]bool) {
	switch typed := value.(type) {
	case string:
		if typed != "" && !isTraceRedactionMarker(typed) {
			result[typed] = true
		}
	case map[string]any:
		for _, child := range typed {
			collectTraceLeafStrings(child, result)
		}
	case []any:
		for _, child := range typed {
			collectTraceLeafStrings(child, result)
		}
	}
}

func redactNormalizedTraceSecrets(trace *NormalizedTrace, secrets []string) {
	if trace == nil || len(secrets) == 0 {
		return
	}
	for index := range trace.Messages {
		trace.Messages[index].Content = redactTraceSecretLiterals(trace.Messages[index].Content, secrets)
		for callIndex := range trace.Messages[index].ToolCalls {
			trace.Messages[index].ToolCalls[callIndex].Arguments = redactTraceSecretLiterals(trace.Messages[index].ToolCalls[callIndex].Arguments, secrets)
		}
	}
	for index := range trace.Actions {
		action := &trace.Actions[index]
		action.Arguments = redactTraceSecretLiterals(action.Arguments, secrets)
		action.Result = redactTraceSecretLiterals(action.Result, secrets)
		action.Error = redactTraceSecretLiterals(action.Error, secrets)
		action.Permission = redactTraceSecretLiterals(action.Permission, secrets)
		for artifactIndex := range action.Artifacts {
			action.Artifacts[artifactIndex].Path = redactTraceSecretLiterals(action.Artifacts[artifactIndex].Path, secrets)
		}
	}
	trace.LastAssistantResult = redactTraceSecretLiterals(trace.LastAssistantResult, secrets)
}

func redactTraceSecretLiterals(value string, secrets []string) string {
	for _, secret := range secrets {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
		if encoded, err := json.Marshal(secret); err == nil && len(encoded) >= 2 {
			value = strings.ReplaceAll(value, string(encoded[1:len(encoded)-1]), "[REDACTED]")
		}
	}
	return value
}

func isTraceRedactionMarker(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "redacted" || value == "[redacted]"
}

func sanitizeArtifacts(artifacts []TraceArtifact) []TraceArtifact {
	if len(artifacts) == 0 {
		return nil
	}
	result := make([]TraceArtifact, len(artifacts))
	copy(result, artifacts)
	for index := range result {
		result[index].Path = sanitizeTraceText(result[index].Path)
		result[index].Kind = sanitizeTraceText(result[index].Kind)
		result[index].SHA256 = sanitizeTraceText(result[index].SHA256)
	}
	return result
}
