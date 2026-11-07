//go:build miniapps

package miniapps

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// DistillInput is the service-facing request for one-session authoring. The
// optional Scenario is supplied after the author confirms a candidate; without
// it Distill returns ScenarioConfirmationError and private evidence.
type DistillInput struct {
	SessionID    string
	Title        string
	Author       string
	Messages     []llm.Message
	Evidence     []TraceCallEvidence
	Trace        *NormalizedTrace
	Scenario     *TraceConfirmedScenario
	ModelBinding *ModelBinding
	FixtureFiles map[string][]byte
	TurnActive   bool
	// CommandProfiles are the profiles available for run_command rewriting
	// and for embedding into the generated document.
	CommandProfiles []cmdprofile.ProfileSpec
}

// Distill creates a conservative draft from successful observed tool calls.
// No failed or missing call is silently promoted into executable workflow.
func Distill(input DistillInput) (MiniApp, SourceEvidence, error) {
	fixtureFiles, err := sanitizeDistillFixtureFiles(input.FixtureFiles)
	if err != nil {
		return MiniApp{}, SourceEvidence{}, err
	}
	trace, err := distillTrace(input)
	if err != nil {
		return MiniApp{}, SourceEvidence{}, err
	}
	eligibility := AssessTraceEligibility(trace, input.TurnActive)
	evidence := SourceEvidence{
		SessionID:      input.SessionID,
		SanitizedTrace: &trace,
		AcceptedResult: trace.LastAssistantResult,
		SourceFixture:  traceFixture(trace),
		FixtureFiles:   fixtureFiles,
		Metrics: map[string]any{
			"tool_calls":            trace.ToolCallCount,
			"successful_tool_calls": trace.SuccessfulToolCallCount,
			"eligibility":           eligibility.Status,
		},
		CreatedAt: time.Now().UTC(),
	}
	if !eligibility.Eligible {
		return MiniApp{}, evidence, fmt.Errorf("session is not distillable: %s", eligibility.Reason)
	}

	candidates := GenerateScenarioCandidates(trace)
	evidence.ScenarioCandidates = candidates
	if input.Scenario == nil {
		return MiniApp{}, evidence, &ScenarioConfirmationError{Candidates: candidates}
	}
	scenario, err := validateDistillScenario(*input.Scenario, trace)
	if err != nil {
		return MiniApp{}, evidence, err
	}
	evidence.ConfirmedScenario = &scenario

	specs := ClassifyTraceInputs(trace, scenario)
	app, err := synthesizeMiniApp(input, trace, scenario, specs)
	if err != nil {
		return MiniApp{}, evidence, err
	}
	return app, evidence, nil
}

// DistillationSummary is a compact status string for job records and logs.
func DistillationSummary(app MiniApp) string {
	return fmt.Sprintf("%s (%d inputs, %d steps)", app.Metadata.Name, len(app.Inputs), len(app.Workflow))
}

// DistillTrace normalizes and checks a source session without requiring
// scenario confirmation. It is useful to render the waiting-for-scenario UI.
func DistillTrace(input TraceInput) (NormalizedTrace, TraceEligibility, []TraceScenarioCandidate, error) {
	trace, err := ExtractNormalizedTrace(input)
	if err != nil {
		return NormalizedTrace{}, TraceEligibility{}, nil, err
	}
	// Rewrite before eligibility and candidates so every later stage —
	// classification, synthesis, evidence, replay — sees the typed form.
	RewriteCommandActions(&trace, input.CommandProfiles)
	eligibility := AssessTraceEligibility(trace, input.TurnActive)
	return trace, eligibility, GenerateScenarioCandidates(trace), nil
}

func distillTrace(input DistillInput) (NormalizedTrace, error) {
	if input.Trace != nil {
		trace := *input.Trace
		if trace.SessionID == "" {
			trace.SessionID = input.SessionID
		}
		return trace, nil
	}
	trace, err := NormalizeSessionTrace(input.SessionID, input.Messages, input.Evidence)
	if err != nil {
		return trace, err
	}
	// Direct Distill() callers skip DistillTrace, so mirror the rewrite here.
	// It is idempotent: rewritten actions no longer carry the run_command name.
	RewriteCommandActions(&trace, input.CommandProfiles)
	return trace, nil
}

func validateDistillScenario(scenario TraceConfirmedScenario, trace NormalizedTrace) (TraceConfirmedScenario, error) {
	if strings.TrimSpace(scenario.Task) == "" || strings.TrimSpace(scenario.AcceptedOutcome) == "" {
		return TraceConfirmedScenario{}, errors.New("confirmed scenario needs a task and accepted outcome")
	}
	if len(scenario.ActionIndexes) == 0 {
		return TraceConfirmedScenario{}, errors.New("confirmed scenario needs at least one action")
	}
	seen := make(map[int]bool, len(scenario.ActionIndexes))
	for _, index := range scenario.ActionIndexes {
		if index < 0 || index >= len(trace.Actions) {
			return TraceConfirmedScenario{}, fmt.Errorf("confirmed scenario action index %d is out of range", index)
		}
		if seen[index] {
			return TraceConfirmedScenario{}, fmt.Errorf("confirmed scenario repeats action index %d", index)
		}
		seen[index] = true
	}
	return scenario, nil
}

func synthesizeMiniApp(input DistillInput, trace NormalizedTrace, scenario TraceConfirmedScenario, specs []TraceInputSpec) (MiniApp, error) {
	name := strings.TrimSpace(input.Title)
	if name == "" {
		name = firstTraceLine(scenario.Task, 64)
	}
	if name == "" {
		name = "Distilled MiniApp"
	}
	appID := portableDistillID(name)
	app := MiniApp{
		SchemaVersion: SchemaVersion,
		Kind:          KindMiniApp,
		ID:            appID,
		State:         StateDraft,
		Metadata: Metadata{
			Name:        name,
			Description: "Reusable workflow distilled from a successful FoxxyCode session.",
			Goal:        firstTraceLine(scenario.Task, 240),
			Author:      strings.TrimSpace(input.Author),
			Tags:        []string{"distilled"},
		},
		Display: DisplaySpec{Title: name, Description: scenario.AcceptedOutcome, Layout: "form-result"},
		Runtime: RuntimePolicy{
			LogScope: "global", OperatorEventLevel: "status",
			DiagnosticToolEvents: "sanitized", PersistAgentReasoning: false,
		},
	}

	inputByID := make(map[string]TraceInputSpec)
	for _, spec := range specs {
		if spec.Class == TraceInputFixed || spec.Class == TraceInputPriorStep || spec.Class == TraceInputSourceSpecific {
			continue
		}
		inputByID[spec.ID] = spec
		app.Inputs = append(app.Inputs, Input{
			ID: spec.ID, Type: generatedInputType(spec), Title: spec.Title,
			Description: spec.Description, Required: spec.Required,
			UI: InputUI{Control: generatedInputControl(spec), Order: len(app.Inputs) * 10},
		})
	}
	if input.ModelBinding != nil {
		app.Requirements.ModelBindings = []ModelBinding{*input.ModelBinding}
		app.Permissions.Models = []string{input.ModelBinding.ID}
	}

	toolSet := make(map[string]bool)
	stepIDByAction := make(map[int]string)
	actionIndexes := append([]int(nil), scenario.ActionIndexes...)
	sort.Ints(actionIndexes)
	for _, actionIndex := range actionIndexes {
		if actionIndex < 0 || actionIndex >= len(trace.Actions) {
			continue
		}
		action := trace.Actions[actionIndex]
		if action.Status != TraceActionSucceeded || action.Orphan || strings.TrimSpace(action.Name) == "" {
			continue
		}
		arguments := decodeTraceArguments(action.Arguments)
		for _, spec := range specs {
			for _, occurrence := range spec.Occurrences {
				if occurrence.ActionIndex != actionIndex || spec.Class == TraceInputFixed || spec.Class == TraceInputSourceSpecific {
					continue
				}
				if spec.Class == TraceInputPriorStep {
					if spec.PriorActionIndex != nil {
						if priorStepID := stepIDByAction[*spec.PriorActionIndex]; priorStepID != "" {
							_ = setTraceReference(arguments, occurrence.JSONPath, Ref{Ref: "steps." + priorStepID + ".outputs.result"})
						}
					}
					continue
				}
				_ = setTraceReference(arguments, occurrence.JSONPath, Ref{Ref: "inputs." + spec.ID})
			}
		}
		stepID := fmt.Sprintf("tool-%d-%s", len(app.Workflow)+1, portableStepSlug(action.Name))
		app.Workflow = append(app.Workflow, Step{
			ID: stepID, Kind: "tool", Title: action.Name,
			Tool: action.Name, Arguments: arguments,
		})
		stepIDByAction[actionIndex] = stepID
		toolSet[action.Name] = true
	}
	if len(app.Workflow) == 0 {
		return MiniApp{}, errors.New("confirmed scenario has no successful tool actions")
	}
	for tool := range toolSet {
		app.Permissions.Tools = append(app.Permissions.Tools, tool)
		// A cmd_* step only runs if the app carries its profile: embed the
		// matched declaration so the document is portable. The profile vanishing
		// between analysis and synthesis would produce an app that can never
		// run, so it is a hard error rather than a silent omission.
		if strings.HasPrefix(tool, "cmd_") {
			profile, declared := commandProfileByToolName(input.CommandProfiles, tool)
			if !declared {
				return MiniApp{}, fmt.Errorf("command profile for %s is no longer available", tool)
			}
			app.Requirements.Commands = append(app.Requirements.Commands, profile.Portable())
		}
	}
	sort.Strings(app.Permissions.Tools)
	sort.Slice(app.Requirements.Commands, func(i, j int) bool {
		return app.Requirements.Commands[i].Name < app.Requirements.Commands[j].Name
	})
	lastStep := app.Workflow[len(app.Workflow)-1].ID
	app.Success = SuccessSpec{
		Mode:   "all",
		Checks: []SuccessCheck{{Kind: "step", Step: lastStep, Status: string(TraceActionSucceeded)}},
	}
	app.Outputs = []Output{{
		ID: "result", Type: "json", Value: Ref{Ref: "steps." + lastStep + ".outputs.result"},
		Renderer: "json", Title: "Result",
	}}
	return app, nil
}

const (
	maxDistillFixtureFileBytes  = 10 << 20
	maxDistillFixtureTotalBytes = 100 << 20
)

var (
	distillFixtureCredentialRE = regexp.MustCompile(`(?i)(auth|authorization|credential|api[_-]?key|access[_-]?token|token|password|secret|cookie)\s*[:=]\s*["']?[A-Za-z0-9._~+/-]{8,}`)
	distillWindowsDrivePathRE  = regexp.MustCompile(`(?i)^[a-z]:/`)
)

func sanitizeDistillFixtureFiles(files map[string][]byte) (map[string][]byte, error) {
	if len(files) == 0 {
		return nil, nil
	}
	result := make(map[string][]byte, len(files))
	total := 0
	for original, content := range files {
		name := strings.ReplaceAll(strings.TrimSpace(original), "\\", "/")
		clean := path.Clean(name)
		if name == "" || strings.ContainsRune(name, '\x00') || path.IsAbs(name) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || distillWindowsDrivePathRE.MatchString(clean) {
			return nil, fmt.Errorf("fixture path %q is not a safe relative path", original)
		}
		if _, exists := result[clean]; exists {
			return nil, fmt.Errorf("fixture path %q duplicates normalized path %q", original, clean)
		}
		if len(content) > maxDistillFixtureFileBytes {
			return nil, fmt.Errorf("fixture file %q exceeds %d bytes", clean, maxDistillFixtureFileBytes)
		}
		total += len(content)
		if total > maxDistillFixtureTotalBytes {
			return nil, fmt.Errorf("fixture files exceed %d bytes", maxDistillFixtureTotalBytes)
		}
		if distillFixtureCredentialRE.Match(content) {
			return nil, fmt.Errorf("fixture file %q contains a possible credential", clean)
		}
		result[clean] = append([]byte(nil), content...)
	}
	if report := SanitizeFiles(result); !report.Clean {
		return nil, fmt.Errorf("fixture sanitization failed: %s", report.Findings[0].Message)
	}
	return result, nil
}

func generatedInputType(spec TraceInputSpec) string {
	if spec.Class == TraceInputSecret {
		return "secret"
	}
	if spec.Type == "number" || spec.Type == "boolean" || spec.Type == "integer" {
		return spec.Type
	}
	return "text"
}

func generatedInputControl(spec TraceInputSpec) string {
	if spec.Class == TraceInputSecret {
		return "password"
	}
	switch spec.Type {
	case "number", "integer":
		return "number"
	case "boolean":
		return "checkbox"
	default:
		return "text"
	}
}

func decodeTraceArguments(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value
	}
	return sanitizeTraceText(raw)
}

// setTraceReference replaces a simple JSON path with a typed $ref object. It
// accepts paths emitted by ClassifyTraceInputs (for example $.query or
// $.files[0].path).
func setTraceReference(root any, path string, reference Ref) bool {
	parts := tracePathParts(path)
	if len(parts) == 0 {
		return false
	}
	return setTracePathValue(root, parts, reference)
}

type tracePathPart struct {
	key   string
	index *int
}

func tracePathParts(path string) []tracePathPart {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "$") {
		return nil
	}
	path = strings.TrimPrefix(path, "$")
	result := make([]tracePathPart, 0)
	for len(path) > 0 {
		switch path[0] {
		case '.':
			path = path[1:]
			end := len(path)
			for index, char := range path {
				if char == '.' || char == '[' {
					end = index
					break
				}
			}
			if end == 0 {
				return nil
			}
			result = append(result, tracePathPart{key: path[:end]})
			path = path[end:]
		case '[':
			end := strings.IndexByte(path, ']')
			if end < 2 {
				return nil
			}
			var index int
			if _, err := fmt.Sscanf(path[1:end], "%d", &index); err != nil || index < 0 {
				return nil
			}
			result = append(result, tracePathPart{index: &index})
			path = path[end+1:]
		default:
			return nil
		}
	}
	return result
}

func setTracePathValue(current any, parts []tracePathPart, value any) bool {
	if len(parts) == 0 {
		return false
	}
	part := parts[0]
	if part.index != nil {
		list, ok := current.([]any)
		if !ok || *part.index < 0 || *part.index >= len(list) {
			return false
		}
		if len(parts) == 1 {
			list[*part.index] = value
			return true
		}
		return setTracePathValue(list[*part.index], parts[1:], value)
	}
	object, ok := current.(map[string]any)
	if !ok {
		return false
	}
	child, exists := object[part.key]
	if !exists {
		return false
	}
	if len(parts) == 1 {
		object[part.key] = value
		return true
	}
	return setTracePathValue(child, parts[1:], value)
}

func traceFixture(trace NormalizedTrace) map[string]any {
	fixture := map[string]any{
		"messages": trace.Messages,
		"actions":  trace.Actions,
	}
	return fixture
}

func firstTraceLine(value string, max int) string {
	value = strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
	if max > 0 {
		runes := []rune(value)
		if len(runes) > max {
			value = strings.TrimSpace(string(runes[:max]))
		}
	}
	return value
}

var distillNonID = regexp.MustCompile(`[^a-z0-9]+`)

func portableDistillID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = distillNonID.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) > 48 {
		value = strings.Trim(value[:48], "-")
	}
	if len(value) < 3 {
		value = "miniapp"
	}
	return value
}

func portableStepSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = distillNonID.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		value = "action"
	}
	if len(value) > 40 {
		value = strings.Trim(value[:40], "-")
	}
	return value
}
