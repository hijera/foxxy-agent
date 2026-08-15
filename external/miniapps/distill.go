//go:build miniapps

package miniapps

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

type DistillInput struct {
	SessionID    string
	Title        string
	Author       string
	Messages     []llm.Message
	ModelBinding *ModelBinding
}

// Distill creates a conservative editable draft from the accepted session
// path. The editor is expected to refine generated controls and steps before
// release; source evidence stays in the private authoring directory.
func Distill(input DistillInput) (MiniApp, SourceEvidence, error) {
	var firstUser, lastAssistant string
	for _, message := range input.Messages {
		switch message.Role {
		case llm.RoleUser:
			if firstUser == "" && strings.TrimSpace(message.Content) != "" {
				firstUser = strings.TrimSpace(message.Content)
			}
		case llm.RoleAssistant:
			if strings.TrimSpace(message.Content) != "" {
				lastAssistant = strings.TrimSpace(message.Content)
			}
		}
	}
	if firstUser == "" || lastAssistant == "" {
		return MiniApp{}, SourceEvidence{}, errorsNotDistillable("the session needs a user task and an accepted assistant result")
	}
	name := strings.TrimSpace(input.Title)
	if name == "" {
		name = firstLine(firstUser, 64)
	}
	id := slugID(name)
	if len(id) < 3 {
		id = "miniapp"
	}
	id += "-" + strings.TrimPrefix(newID("d"), "d-")[:6]

	app := MiniApp{
		SchemaVersion: SchemaVersion,
		Kind:          KindMiniApp,
		ID:            id,
		State:         StateDraft,
		Metadata: Metadata{
			Name: name, Description: "Reusable workflow distilled from a successful FoxxyCode session.",
			Goal: firstLine(firstUser, 240), Author: strings.TrimSpace(input.Author),
			Tags: []string{"distilled"},
		},
		Inputs: []Input{{
			ID: "request", Type: "text", Title: "Task input",
			Description: "Adjust the session-derived task for this run.", Required: true,
			Default: firstUser, UI: InputUI{Control: "textarea", Order: 10},
		}},
		Success: SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
			Kind: "step", Step: "execute_task", Status: "succeeded",
		}}},
		Outputs: []Output{{
			ID: "result", Type: "markdown", Value: Ref{Ref: "steps.execute_task.outputs.result"},
			Renderer: "markdown", Title: "Result",
		}},
		Display: DisplaySpec{Title: name, Layout: "form-result"},
		Runtime: RuntimePolicy{
			LogScope: "global", OperatorEventLevel: "status",
			DiagnosticToolEvents: "sanitized", PersistAgentReasoning: false,
		},
	}
	if input.ModelBinding != nil {
		binding := *input.ModelBinding
		app.Requirements.ModelBindings = []ModelBinding{binding}
		app.Permissions.Models = []string{binding.ID}
		app.Workflow = []Step{{
			ID: "execute_task", Kind: "agent", Title: "Execute distilled task",
			Prompt:       "Reproduce the successful workflow for this operator input:\n\n{{ inputs.request }}",
			ModelBinding: binding.ID, TimeoutSeconds: 600,
		}}
	} else {
		app.Workflow = []Step{{
			ID: "execute_task", Kind: "program", Title: "Return accepted session result",
			Language: VMVersion, Entry: "main",
			Functions: map[string][]Instruction{"main": {
				{Op: "const", Arg: lastAssistant},
				{Op: "return"},
			}},
			Limits: ProgramLimits{Instructions: 100, WallTimeSeconds: 5, StackDepth: 16, CallDepth: 4},
		}}
	}
	evidence := SourceEvidence{
		SessionID: input.SessionID, SanitizedUser: sanitizeText(firstUser),
		AcceptedResult: sanitizeText(lastAssistant), CreatedAt: time.Now().UTC(),
	}
	return app, evidence, nil
}

type notDistillableError struct{ reason string }

func (e notDistillableError) Error() string { return "session is not distillable: " + e.reason }
func errorsNotDistillable(reason string) error {
	return notDistillableError{reason: reason}
}

func firstLine(value string, max int) string {
	value = strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
	runes := []rune(value)
	if len(runes) > max {
		value = strings.TrimSpace(string(runes[:max])) + "…"
	}
	return value
}

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

func slugID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonID.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) > 48 {
		value = strings.Trim(value[:48], "-")
	}
	return value
}

func sanitizeText(value string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*\S+`),
		regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/-]+=*`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{12,}\b`),
	}
	for _, pattern := range patterns {
		value = pattern.ReplaceAllString(value, "[REDACTED]")
	}
	return value
}

func DistillationSummary(app MiniApp) string {
	return fmt.Sprintf("%s (%d inputs, %d steps)", app.Metadata.Name, len(app.Inputs), len(app.Workflow))
}
