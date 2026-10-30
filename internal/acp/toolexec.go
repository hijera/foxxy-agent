package acp

import "context"

// Tool execution statuses returned by ExecuteToolCall.
const (
	// ToolExecHandled: the client executed the call; Output or Err carries the
	// tool result the model must see.
	ToolExecHandled = "handled"
	// ToolExecNotHandled: the client declined the call (or none claimed it);
	// the agent executes it locally.
	ToolExecNotHandled = "not_handled"
)

// ToolExecutionRequest is one tool call offered to a connected client for
// execution there - for example an IDE plugin applying write/edit/apply_patch
// through the editor's document model instead of the agent writing the disk.
// The call is offered by name alone: built-in and MCP tool names reach the
// client exactly as the model called them.
type ToolExecutionRequest struct {
	SessionID  string
	ToolCallID string
	ToolName   string
	ArgsJSON   string
}

// ToolExecutionResult is the client's verdict on a ToolExecutionRequest.
type ToolExecutionResult struct {
	// Status is ToolExecHandled or ToolExecNotHandled.
	Status string
	// Output is the tool result when Status is ToolExecHandled and the call
	// succeeded.
	Output string
	// Err is the error text when Status is ToolExecHandled and the call
	// failed; the agent reports it to the model as the tool error.
	Err string
}

// ToolExecutor is optionally implemented by an UpdateSender that can route a
// tool call to a connected client and wait for the client's result. The agent
// type-asserts the sender for it before dispatching a call locally, so a
// surface with no client behind it simply runs tools itself.
type ToolExecutor interface {
	ExecuteToolCall(ctx context.Context, req ToolExecutionRequest) (ToolExecutionResult, error)
}
