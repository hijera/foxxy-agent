//go:build miniapps

package miniapps

import (
	"github.com/hijera/foxxycode-agent/internal/cmdprofile"

	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// MissingInputError distinguishes an incomplete launch form from an invalid
// workflow or a failed execution. HTTP/UI adapters can resume collection
// without presenting the run as failed.
type MissingInputError struct {
	InputID string
}

func (e *MissingInputError) Error() string { return fmt.Sprintf("input %q is required", e.InputID) }

func IsMissingInput(err error) bool {
	var target *MissingInputError
	return errors.As(err, &target)
}

var portableIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}[a-z0-9]$`)

var supportedInputTypes = map[string]bool{
	"string": true, "text": true, "integer": true, "number": true,
	"boolean": true, "date": true, "datetime": true, "enum": true,
	"file": true, "files": true, "directory": true, "secret": true,
}

var supportedStepKinds = map[string]bool{
	"tool": true, "llm": true, "agent": true,
	"confirm": true, "branch": true, "miniapp": true,
}

var supportedInputControls = map[string]bool{
	"text": true, "textarea": true, "number": true, "checkbox": true,
	"select": true, "radio": true, "date": true, "datetime": true,
	"file": true, "files": true, "directory": true, "password": true,
}

var allowedRunStatuses = map[string]bool{
	string(RunPending): true, string(RunRunning): true,
	string(RunWaitingForInput): true, string(RunWaitingForConfirmation): true,
	string(RunSucceeded): true, string(RunFailed): true,
	string(RunCancelled): true, string(RunInterrupted): true,
}

// Validate performs schema and reference validation without requiring a live
// tool registry. Use ValidateWithCapabilities when registry/model catalogs
// are available at preflight time.
func Validate(app MiniApp) ValidationReport {
	return validate(app, CapabilitySet{})
}

// ValidateWithCapabilities additionally verifies that every declared tool,
// model, and nested released app exists in the current host catalog.
func ValidateWithCapabilities(app MiniApp, capabilities CapabilitySet) ValidationReport {
	return validate(app, capabilities)
}

func validate(app MiniApp, capabilities CapabilitySet) ValidationReport {
	issues := make([]ValidationIssue, 0)
	add := func(path, message string) {
		issues = append(issues, ValidationIssue{Path: path, Severity: "error", Message: message})
	}

	if app.SchemaVersion != SchemaVersion {
		add("schema_version", "must be "+SchemaVersion)
	}
	if app.Kind != KindMiniApp {
		add("kind", "must be "+KindMiniApp)
	}
	if !portableIDPattern.MatchString(app.ID) {
		add("id", "must be a portable lower-case identifier with 3-64 characters")
	}
	switch app.State {
	case StateDraft:
		if app.Version != "" {
			add("version", "drafts must not carry a release version")
		}
	case StateReleased:
		if !validSemanticVersion(app.Version) {
			add("version", "released apps require MAJOR.MINOR.PATCH")
		}
	default:
		add("state", "must be draft or released")
	}
	if strings.TrimSpace(app.Metadata.Name) == "" {
		add("metadata.name", "is required")
	}
	if strings.TrimSpace(app.Metadata.Goal) == "" {
		add("metadata.goal", "is required")
	}
	if len(app.Workflow) == 0 {
		add("workflow", "must contain at least one step")
	}
	if app.Runtime.LogScope != "local" && app.Runtime.LogScope != "global" {
		add("runtime.log_scope", "must be local or global")
	}
	if app.Runtime.OperatorEventLevel != "status" {
		add("runtime.operator_event_level", "must be status in schema v1")
	}
	switch app.Runtime.DiagnosticToolEvents {
	case "none", "metadata", "sanitized":
	default:
		add("runtime.diagnostic_tool_events", "must be none, metadata, or sanitized")
	}
	if app.Runtime.PersistAgentReasoning {
		add("runtime.persist_agent_reasoning", "must be false in schema v1")
	}

	commandTools := make(map[string]bool, len(app.Requirements.Commands))
	for index := range app.Requirements.Commands {
		profile := app.Requirements.Commands[index]
		path := fmt.Sprintf("requirements.commands[%d]", index)
		if err := profile.Validate(); err != nil {
			add(path, err.Error())
			continue
		}
		if commandTools[profile.ToolName()] {
			add(path+".name", "is duplicated")
		}
		commandTools[profile.ToolName()] = true
		// Mini Apps run unattended; an ask-profile could never be approved
		// mid-run, so only allow-profiles may be embedded.
		if profile.ResolvedPermission() != cmdprofile.PermissionAllow {
			add(path+".permission", "embedded command profiles must declare permission: allow")
		}
	}

	modelIDs := make(map[string]bool, len(app.Requirements.ModelBindings))
	for index, binding := range app.Requirements.ModelBindings {
		path := fmt.Sprintf("requirements.model_bindings[%d]", index)
		if !portableIDPattern.MatchString(binding.ID) {
			add(path+".id", "must be a portable identifier")
		}
		if modelIDs[binding.ID] {
			add(path+".id", "is duplicated")
		}
		modelIDs[binding.ID] = true
		if strings.TrimSpace(binding.Selection) == "" {
			add(path+".selection", "is required")
		}
		if binding.Selection == "fixed" {
			if strings.TrimSpace(binding.Provider.BaseURL) == "" {
				add(path+".provider.base_url", "is required for a fixed binding")
			}
			if strings.TrimSpace(binding.Model) == "" {
				add(path+".model", "is required for a fixed binding")
			}
		}
		if capabilities.Models != nil && !capabilities.Models[binding.ID] && !capabilities.Models[binding.Selection] {
			add(path+".id", "model binding is unavailable")
		}
	}

	toolPermissions := make(map[string]bool, len(app.Permissions.Tools))
	for index, tool := range app.Permissions.Tools {
		path := fmt.Sprintf("permissions.tools[%d]", index)
		if strings.TrimSpace(tool) == "" {
			add(path, "must not be empty")
		}
		if toolPermissions[tool] {
			add(path, "is duplicated")
		}
		toolPermissions[tool] = true
	}
	for index, model := range app.Permissions.Models {
		if !modelIDs[model] {
			add(fmt.Sprintf("permissions.models[%d]", index), "must reference a declared model binding")
		}
	}

	inputIDs := make(map[string]bool, len(app.Inputs))
	secretIDs := make(map[string]bool)
	for _, input := range app.Inputs {
		inputIDs[input.ID] = true
		if input.Type == "secret" {
			secretIDs[input.ID] = true
		}
	}
	seenInputIDs := make(map[string]bool, len(app.Inputs))
	for index, input := range app.Inputs {
		path := fmt.Sprintf("inputs[%d]", index)
		if !portableIDPattern.MatchString(input.ID) {
			add(path+".id", "must be a portable identifier")
		}
		if seenInputIDs[input.ID] {
			add(path+".id", "is duplicated")
		}
		seenInputIDs[input.ID] = true
		if !supportedInputTypes[input.Type] {
			add(path+".type", "is unsupported")
		}
		if !supportedInputControls[input.UI.Control] {
			add(path+".ui.control", "is unsupported")
		}
		if input.Type == "secret" {
			if input.Default != nil && strings.TrimSpace(fmt.Sprint(input.Default)) != "" {
				add(path+".default", "secret inputs cannot have persisted defaults")
			}
		}
		validateInputValidation(input.Validation, path+".validation", add)
		for name, condition := range map[string]*Condition{
			"visible_when": input.VisibleWhen, "enabled_when": input.EnabledWhen,
			"required_when": input.RequiredWhen,
		} {
			if condition != nil {
				validateCondition(*condition, path+"."+name, add, refContext{
					inputIDs: inputIDs, secretIDs: secretIDs, stepIDs: nil,
				})
			}
		}
	}
	if cycle := inputDependencyCycle(app.Inputs); len(cycle) > 0 {
		add("inputs", "dependency graph contains a cycle: "+strings.Join(cycle, " -> "))
	}

	allStepIDs := make(map[string]bool)
	var collectStepIDs func([]Step, string)
	collectStepIDs = func(steps []Step, base string) {
		for index, step := range steps {
			path := fmt.Sprintf("%s[%d]", base, index)
			if !portableIDPattern.MatchString(step.ID) {
				add(path+".id", "must be a portable identifier")
			}
			if allStepIDs[step.ID] {
				add(path+".id", "is duplicated")
			}
			allStepIDs[step.ID] = true
			collectStepIDs(step.Then, path+".then")
			collectStepIDs(step.Else, path+".else")
		}
	}
	collectStepIDs(app.Workflow, "workflow")

	stepContext := refContext{inputIDs: inputIDs, secretIDs: secretIDs, stepIDs: allStepIDs}
	seenSteps := make(map[string]bool)
	var validateSteps func([]Step, string, map[string]bool)
	validateSteps = func(steps []Step, base string, prior map[string]bool) {
		for index, step := range steps {
			path := fmt.Sprintf("%s[%d]", base, index)
			if !supportedStepKinds[step.Kind] {
				add(path+".kind", "is unsupported")
			}
			if step.TimeoutSeconds < 0 {
				add(path+".timeout_seconds", "must not be negative")
			}
			if step.Retry.MaxAttempts < 0 || step.Retry.DelayMS < 0 {
				add(path+".retry", "retry limits must not be negative")
			}
			if step.Retry.MaxAttempts > 32 {
				add(path+".retry.max_attempts", "must not exceed 32")
			}
			if step.When != nil {
				validateCondition(*step.When, path+".when", add, stepContext.withPriorSteps(prior))
			}
			switch step.Kind {
			case "tool":
				if strings.TrimSpace(step.Tool) == "" {
					add(path+".tool", "is required")
				} else {
					if !toolPermissions[step.Tool] {
						add(path+".tool", "must be declared in permissions.tools")
					}
					if capabilities.Tools != nil && !capabilities.Tools[step.Tool] {
						add(path+".tool", "tool is unavailable")
					}
					// A command-profile step needs its declaration to travel with
					// the document; without it the app cannot run anywhere else.
					if strings.HasPrefix(step.Tool, "cmd_") && !commandTools[step.Tool] &&
						(capabilities.Tools == nil || !capabilities.Tools[step.Tool]) {
						add(path+".tool", "command profile must be declared in requirements.commands")
					}
				}
				validateStepValue(step.Arguments, path+".arguments", add, stepContext.withPriorSteps(prior))
				validateToolShape(step, path, add)
			case "llm":
				if strings.TrimSpace(step.Prompt) == "" {
					add(path+".prompt", "is required")
				}
				validateModelReference(step.ModelBinding, path+".model_binding", modelIDs, app.Requirements.ModelBindings, app.Permissions.Models, capabilities, add)
				validateStepValue(step.Prompt, path+".prompt", add, stepContext.withPriorSteps(prior))
				validateStepValue(step.OutputSchema, path+".output_schema", add, stepContext.withPriorSteps(prior))
				validateOutputSchema(step.OutputSchema, path+".output_schema", add)
				validateLLMShape(step, path, add)
			case "agent":
				if strings.TrimSpace(step.Prompt) == "" {
					add(path+".prompt", "is required")
				}
				if step.MaxTurns < 1 || step.MaxTurns > 32 {
					add(path+".max_turns", "must be between 1 and 32")
				}
				validateModelReference(step.ModelBinding, path+".model_binding", modelIDs, app.Requirements.ModelBindings, app.Permissions.Models, capabilities, add)
				for toolIndex, tool := range step.Tools {
					if !toolPermissions[tool] {
						add(fmt.Sprintf("%s.tools[%d]", path, toolIndex), "must be declared in permissions.tools")
					}
					if capabilities.Tools != nil && !capabilities.Tools[tool] {
						add(fmt.Sprintf("%s.tools[%d]", path, toolIndex), "tool is unavailable")
					}
				}
				validateStepValue(step.Prompt, path+".prompt", add, stepContext.withPriorSteps(prior))
				validateStepValue(step.OutputSchema, path+".output_schema", add, stepContext.withPriorSteps(prior))
				validateOutputSchema(step.OutputSchema, path+".output_schema", add)
				validateAgentShape(step, path, add)
			case "confirm":
				if strings.TrimSpace(step.Message) == "" {
					add(path+".message", "is required")
				}
				validateStepValue(step.Message, path+".message", add, stepContext.withPriorSteps(prior))
				validateStepValue(step.Details, path+".details", add, stepContext.withPriorSteps(prior))
				validateConfirmShape(step, path, add)
			case "branch":
				if step.If == nil {
					add(path+".if", "is required")
				} else {
					validateCondition(*step.If, path+".if", add, stepContext.withPriorSteps(prior))
				}
				validateBranchShape(step, path, add)
				validateSteps(step.Then, path+".then", cloneStepSet(prior))
				validateSteps(step.Else, path+".else", cloneStepSet(prior))
			case "miniapp":
				if !portableIDPattern.MatchString(step.AppID) {
					add(path+".app_id", "must be a portable identifier")
				}
				if !validSemanticVersion(step.AppVersion) {
					add(path+".app_version", "must be MAJOR.MINOR.PATCH")
				}
				if len(app.Permissions.Apps) > 0 && !containsStringCore(app.Permissions.Apps, step.AppID) {
					add(path+".app_id", "must be declared in permissions.apps")
				}
				if capabilities.Apps != nil && !capabilities.Apps[step.AppID+"@"+step.AppVersion] && !capabilities.Apps[step.AppID] {
					add(path+".app_id", "nested release is unavailable")
				}
				validateStepValue(step.InputMap, path+".inputs", add, stepContext.withPriorSteps(prior))
				validateMiniAppShape(step, path, add)
			}
			prior[step.ID] = true
		}
	}
	validateSteps(app.Workflow, "workflow", seenSteps)

	outputIDs := make(map[string]bool, len(app.Outputs))
	for index, output := range app.Outputs {
		path := fmt.Sprintf("outputs[%d]", index)
		if !portableIDPattern.MatchString(output.ID) {
			add(path+".id", "must be a portable identifier")
		}
		if outputIDs[output.ID] {
			add(path+".id", "is duplicated")
		}
		outputIDs[output.ID] = true
		if strings.TrimSpace(output.Type) == "" {
			add(path+".type", "is required")
		}
		validateStepValue(output.Value, path+".value", add, stepContext.withPriorSteps(seenSteps))
	}

	if len(app.Success.Checks) == 0 {
		add("success.checks", "must contain at least one check")
	}
	if app.Success.Mode != "all" && app.Success.Mode != "any" {
		add("success.mode", "must be all or any")
	}
	for index, check := range app.Success.Checks {
		path := fmt.Sprintf("success.checks[%d]", index)
		switch check.Kind {
		case "step":
			if !seenSteps[check.Step] {
				add(path+".step", "must reference a step that runs on every branch")
			}
			if !allowedRunStatuses[check.Status] {
				add(path+".status", "is unsupported")
			}
		case "schema":
			if check.Value == nil || len(check.Schema) == 0 {
				add(path, "value and schema are required")
			}
			validateStepValue(check.Value, path+".value", add, stepContext.withPriorSteps(seenSteps))
			validateOutputSchema(check.Schema, path+".schema", add)
		case "prompt":
			if check.Value == nil || strings.TrimSpace(check.Prompt) == "" {
				add(path, "value and prompt are required")
			}
			validateModelReference(check.ModelBinding, path+".model_binding", modelIDs, app.Requirements.ModelBindings, app.Permissions.Models, capabilities, add)
			validateStepValue(check.Value, path+".value", add, stepContext.withPriorSteps(seenSteps))
		default:
			add(path+".kind", "is unsupported")
		}
	}

	sort.SliceStable(issues, func(left, right int) bool {
		if issues[left].Path == issues[right].Path {
			return issues[left].Message < issues[right].Message
		}
		return issues[left].Path < issues[right].Path
	})
	return ValidationReport{Valid: len(issues) == 0, Issues: issues}
}

type refContext struct {
	inputIDs   map[string]bool
	secretIDs  map[string]bool
	stepIDs    map[string]bool
	priorSteps map[string]bool
}

func (c refContext) withPriorSteps(prior map[string]bool) refContext {
	c.priorSteps = prior
	return c
}

func validateInputValidation(spec Validation, path string, add func(string, string)) {
	if spec.Minimum != nil && spec.Maximum != nil && *spec.Minimum > *spec.Maximum {
		add(path, "minimum must not exceed maximum")
	}
	if spec.MinLength != nil && *spec.MinLength < 0 {
		add(path+".min_length", "must not be negative")
	}
	if spec.MaxLength != nil && *spec.MaxLength < 0 {
		add(path+".max_length", "must not be negative")
	}
	if spec.MinLength != nil && spec.MaxLength != nil && *spec.MinLength > *spec.MaxLength {
		add(path, "min_length must not exceed max_length")
	}
	if spec.MaxFiles != nil && *spec.MaxFiles < 0 {
		add(path+".max_files", "must not be negative")
	}
	if spec.MaxTotalBytes != nil && *spec.MaxTotalBytes < 0 {
		add(path+".max_total_bytes", "must not be negative")
	}
	if len(spec.Extensions) > 0 || len(spec.MediaTypes) > 0 || spec.MaxFiles != nil || spec.MaxTotalBytes != nil || spec.MustExist || spec.FilesystemKind != "" {
		add(path, "file constraints are not supported in schema v1")
	}
	if spec.Pattern != "" {
		if len(spec.Pattern) > 4096 {
			add(path+".pattern", "must not exceed 4096 bytes")
		} else if _, err := regexp.Compile(spec.Pattern); err != nil {
			add(path+".pattern", "is not a valid regular expression")
		}
	}
}

func validateOutputSchema(schema map[string]any, path string, add func(string, string)) {
	if len(schema) == 0 {
		return
	}
	allowed := map[string]bool{
		"type": true, "required": true, "properties": true, "items": true,
		"enum": true, "minimum": true, "maximum": true, "minLength": true, "maxLength": true,
	}
	for key := range schema {
		if !allowed[key] {
			add(path+"."+key, "is not supported in schema v1")
		}
	}
	if typeName, ok := schema["type"].(string); ok {
		switch typeName {
		case "null", "boolean", "string", "number", "integer", "array", "object":
		default:
			add(path+".type", "is not a supported JSON type")
		}
	} else if _, exists := schema["type"]; exists {
		add(path+".type", "must be a string")
	}
	if properties, exists := schema["properties"]; exists {
		object, ok := properties.(map[string]any)
		if !ok {
			add(path+".properties", "must be an object")
		} else {
			for name, child := range object {
				childSchema, ok := child.(map[string]any)
				if !ok {
					add(path+".properties."+name, "must be an object")
					continue
				}
				validateOutputSchema(childSchema, path+".properties."+name, add)
			}
		}
	}
	if items, exists := schema["items"]; exists {
		childSchema, ok := items.(map[string]any)
		if !ok {
			add(path+".items", "must be an object")
		} else {
			validateOutputSchema(childSchema, path+".items", add)
		}
	}
}

func validateModelReference(id, path string, modelIDs map[string]bool, bindings []ModelBinding, permissions []string, capabilities CapabilitySet, add func(string, string)) {
	if strings.TrimSpace(id) == "" {
		add(path, "is required")
		return
	}
	if !modelIDs[id] {
		add(path, "must reference a declared model binding")
	}
	permissionOK := containsStringCore(permissions, id)
	for _, binding := range bindings {
		if binding.ID == id && containsStringCore(permissions, binding.Selection) {
			permissionOK = true
		}
	}
	if len(permissions) > 0 && !permissionOK {
		add(path, "must be declared in permissions.models")
	}
	capabilityOK := capabilities.Models == nil
	if capabilities.Models != nil {
		capabilityOK = capabilities.Models[id]
		for _, binding := range bindings {
			if binding.ID == id && capabilities.Models[binding.Selection] {
				capabilityOK = true
			}
		}
	}
	if !capabilityOK {
		add(path, "model binding is unavailable")
	}
}

func validateToolShape(step Step, path string, add func(string, string)) {
	if step.Prompt != "" || step.ModelBinding != "" || len(step.Tools) > 0 || step.MaxTurns != 0 || step.OutputSchema != nil || step.Message != "" || step.Details != nil || step.If != nil || len(step.Then) > 0 || len(step.Else) > 0 || step.AppID != "" || step.AppVersion != "" || step.InputMap != nil {
		add(path, "contains fields that are not valid for tool steps")
	}
}

func validateLLMShape(step Step, path string, add func(string, string)) {
	if step.Tool != "" || step.Arguments != nil || len(step.Tools) > 0 || step.MaxTurns != 0 || step.Message != "" || step.Details != nil || step.If != nil || len(step.Then) > 0 || len(step.Else) > 0 || step.AppID != "" || step.AppVersion != "" || step.InputMap != nil {
		add(path, "contains fields that are not valid for llm steps")
	}
}

func validateAgentShape(step Step, path string, add func(string, string)) {
	if step.Tool != "" || step.Arguments != nil || step.Message != "" || step.Details != nil || step.If != nil || len(step.Then) > 0 || len(step.Else) > 0 || step.AppID != "" || step.AppVersion != "" || step.InputMap != nil {
		add(path, "contains fields that are not valid for agent steps")
	}
}

func validateConfirmShape(step Step, path string, add func(string, string)) {
	if step.Tool != "" || step.Arguments != nil || step.Prompt != "" || step.ModelBinding != "" || len(step.Tools) > 0 || step.MaxTurns != 0 || step.OutputSchema != nil || step.If != nil || len(step.Then) > 0 || len(step.Else) > 0 || step.AppID != "" || step.AppVersion != "" || step.InputMap != nil {
		add(path, "contains fields that are not valid for confirm steps")
	}
}

func validateBranchShape(step Step, path string, add func(string, string)) {
	if step.Tool != "" || step.Arguments != nil || step.Prompt != "" || step.ModelBinding != "" || len(step.Tools) > 0 || step.MaxTurns != 0 || step.OutputSchema != nil || step.Message != "" || step.Details != nil || step.AppID != "" || step.AppVersion != "" || step.InputMap != nil {
		add(path, "contains fields that are not valid for branch steps")
	}
}

func validateMiniAppShape(step Step, path string, add func(string, string)) {
	if step.Tool != "" || step.Arguments != nil || step.Prompt != "" || step.ModelBinding != "" || len(step.Tools) > 0 || step.MaxTurns != 0 || step.OutputSchema != nil || step.Message != "" || step.Details != nil || step.If != nil || len(step.Then) > 0 || len(step.Else) > 0 {
		add(path, "contains fields that are not valid for miniapp steps")
	}
}

func validateStepValue(value any, path string, add func(string, string), context refContext) {
	if value == nil {
		return
	}
	switch typed := value.(type) {
	case Ref:
		validateReference(typed.Ref, path, add, context)
	case *Ref:
		if typed == nil {
			add(path, "must not be a nil reference")
			return
		}
		validateReference(typed.Ref, path, add, context)
	case string:
		for _, match := range templatePattern.FindAllStringSubmatch(typed, -1) {
			validateReference(strings.TrimSpace(match[1]), path, add, context)
		}
	case map[string]any:
		if raw, exists := typed["$ref"]; exists {
			ref, ok := raw.(string)
			if !ok || len(typed) != 1 {
				add(path, "$ref objects must contain only a string $ref")
			} else {
				validateReference(ref, path, add, context)
			}
			return
		}
		for key, item := range typed {
			validateStepValue(item, path+"."+key, add, context)
		}
	case []any:
		for index, item := range typed {
			validateStepValue(item, fmt.Sprintf("%s[%d]", path, index), add, context)
		}
	case []string:
		// String slices occur in decoded Go values supplied by adapters.
		for index, item := range typed {
			validateStepValue(item, fmt.Sprintf("%s[%d]", path, index), add, context)
		}
	}
}

func validateReference(path, issuePath string, add func(string, string), context refContext) {
	path = strings.TrimSpace(path)
	parts := strings.Split(path, ".")
	if len(parts) < 2 || path == "" {
		add(issuePath, "reference must use a declared root")
		return
	}
	switch parts[0] {
	case "inputs":
		if !context.inputIDs[parts[1]] {
			add(issuePath, "references unknown input "+parts[1])
		}
	case "secrets":
		if !context.secretIDs[parts[1]] {
			add(issuePath, "references unknown secret "+parts[1])
		}
	case "steps":
		if len(parts) < 4 || parts[1] == "" || parts[2] != "outputs" || parts[3] == "" {
			add(issuePath, "step references must use steps.<id>.outputs.<name>")
			return
		}
		if !context.stepIDs[parts[1]] {
			add(issuePath, "references unknown step "+parts[1])
		} else if context.priorSteps != nil && !context.priorSteps[parts[1]] {
			add(issuePath, "references a step that has not executed yet: "+parts[1])
		}
	case "run":
		if parts[1] != "id" && parts[1] != "workspace" && parts[1] != "output_dir" {
			add(issuePath, "references unknown run value "+parts[1])
		}
	case "app":
		if parts[1] != "id" && parts[1] != "version" {
			add(issuePath, "references unknown app value "+parts[1])
		}
	default:
		add(issuePath, "references unknown root "+parts[0])
	}
}

func validateCondition(condition Condition, path string, add func(string, string), context refContext) {
	if strings.TrimSpace(condition.Op) == "" {
		add(path+".op", "is required")
		return
	}
	switch condition.Op {
	case "and", "or":
		if len(condition.Args) == 0 {
			add(path+".args", "requires at least one condition")
		}
		if condition.Left != nil || condition.Right != nil || condition.Value != nil {
			add(path, "logical conditions cannot have left/right/value")
		}
		for index, child := range condition.Args {
			validateCondition(child, fmt.Sprintf("%s.args[%d]", path, index), add, context)
		}
	case "not":
		if len(condition.Args) != 1 {
			add(path+".args", "not requires exactly one condition")
		}
		for index, child := range condition.Args {
			validateCondition(child, fmt.Sprintf("%s.args[%d]", path, index), add, context)
		}
	case "exists", "empty":
		if condition.Left == nil && condition.Value == nil {
			add(path, "requires value or left")
		}
		if condition.Right != nil {
			add(path+".right", "is not valid for this operator")
		}
		validateStepValue(condition.Left, path+".left", add, context)
		validateStepValue(condition.Value, path+".value", add, context)
	case "eq", "ne", "gt", "gte", "lt", "lte", "contains", "matches":
		if condition.Left == nil && condition.Value == nil {
			add(path, "requires left or value")
		}
		if condition.Right == nil {
			add(path+".right", "is required")
		}
		validateStepValue(condition.Left, path+".left", add, context)
		validateStepValue(condition.Right, path+".right", add, context)
		validateStepValue(condition.Value, path+".value", add, context)
		if condition.Op == "matches" {
			if pattern, ok := condition.Right.(string); ok {
				if len(pattern) > 4096 {
					add(path+".right", "pattern exceeds 4096 bytes")
				} else if _, err := regexp.Compile(pattern); err != nil {
					add(path+".right", "pattern is not a valid regular expression")
				}
			}
		}
	default:
		add(path+".op", "is unsupported")
	}
}

func cloneStepSet(source map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(source))
	for id, present := range source {
		cloned[id] = present
	}
	return cloned
}

func validateInputs(spec []Input, values map[string]any, refs map[string]any) error {
	for _, input := range spec {
		visible, err := inputConditionValue(input.VisibleWhen, refs, true)
		if err != nil {
			return fmt.Errorf("input %q visible_when: %w", input.ID, err)
		}
		enabled, err := inputConditionValue(input.EnabledWhen, refs, true)
		if err != nil {
			return fmt.Errorf("input %q enabled_when: %w", input.ID, err)
		}
		if !visible || !enabled {
			continue
		}
		required, err := inputConditionValue(input.RequiredWhen, refs, false)
		if err != nil {
			return fmt.Errorf("input %q required_when: %w", input.ID, err)
		}
		value, exists := values[input.ID]
		if !exists || value == nil || (input.Type != "boolean" && fmt.Sprint(value) == "") {
			if input.Required || required {
				return &MissingInputError{InputID: input.ID}
			}
			continue
		}
		switch input.Type {
		case "string", "text", "date", "datetime", "file", "directory", "secret", "enum":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("input %q must be a string", input.ID)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("input %q must be a boolean", input.ID)
			}
		case "integer":
			n, ok := number(value)
			if !ok || n != float64(int64(n)) {
				return fmt.Errorf("input %q must be an integer", input.ID)
			}
		case "number":
			if _, ok := number(value); !ok {
				return fmt.Errorf("input %q must be a number", input.ID)
			}
		case "files":
			reflected := reflect.ValueOf(value)
			if !reflected.IsValid() || (reflected.Kind() != reflect.Array && reflected.Kind() != reflect.Slice) {
				return fmt.Errorf("input %q must be an array", input.ID)
			}
		}
		if len(input.Validation.Enum) > 0 && !containsJSON(input.Validation.Enum, value) {
			return fmt.Errorf("input %q is not an allowed value", input.ID)
		}
		if numeric, ok := number(value); ok {
			if input.Validation.Minimum != nil && numeric < *input.Validation.Minimum {
				return fmt.Errorf("input %q is below its minimum", input.ID)
			}
			if input.Validation.Maximum != nil && numeric > *input.Validation.Maximum {
				return fmt.Errorf("input %q exceeds its maximum", input.ID)
			}
		}
		if stringValue, ok := value.(string); ok {
			if input.Validation.MinLength != nil && len([]rune(stringValue)) < *input.Validation.MinLength {
				return fmt.Errorf("input %q is too short", input.ID)
			}
			if input.Validation.MaxLength != nil && len([]rune(stringValue)) > *input.Validation.MaxLength {
				return fmt.Errorf("input %q is too long", input.ID)
			}
			if input.Validation.Pattern != "" {
				matched, _ := regexp.MatchString(input.Validation.Pattern, stringValue)
				if !matched {
					return fmt.Errorf("input %q does not match its pattern", input.ID)
				}
			}
		}
	}
	return nil
}

func inputConditionValue(condition *Condition, refs map[string]any, fallback bool) (bool, error) {
	if condition == nil {
		return fallback, nil
	}
	return evaluateCondition(*condition, refs)
}

func inputDependencyCycle(inputs []Input) []string {
	graph := make(map[string][]string, len(inputs))
	known := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		known[input.ID] = true
	}
	for _, input := range inputs {
		dependencies := map[string]bool{}
		for _, condition := range []*Condition{input.VisibleWhen, input.EnabledWhen, input.RequiredWhen} {
			collectInputConditionRefs(condition, dependencies)
		}
		for dependency := range dependencies {
			if dependency == input.ID {
				return []string{input.ID, input.ID}
			}
			if known[dependency] {
				graph[input.ID] = append(graph[input.ID], dependency)
			}
		}
		sort.Strings(graph[input.ID])
	}
	state := make(map[string]int)
	stack := make([]string, 0, len(inputs))
	var visit func(string) []string
	visit = func(id string) []string {
		state[id] = 1
		stack = append(stack, id)
		for _, dependency := range graph[id] {
			switch state[dependency] {
			case 0:
				if cycle := visit(dependency); len(cycle) > 0 {
					return cycle
				}
			case 1:
				start := 0
				for start < len(stack) && stack[start] != dependency {
					start++
				}
				if start < len(stack) {
					return append(append([]string{}, stack[start:]...), dependency)
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		return nil
	}
	for id := range known {
		if state[id] == 0 {
			if cycle := visit(id); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}

func collectInputConditionRefs(condition *Condition, found map[string]bool) {
	if condition == nil {
		return
	}
	for _, value := range []any{condition.Left, condition.Right, condition.Value} {
		path := ""
		switch typed := value.(type) {
		case Ref:
			path = typed.Ref
		case *Ref:
			if typed != nil {
				path = typed.Ref
			}
		case map[string]any:
			path, _ = typed["$ref"].(string)
		}
		if strings.HasPrefix(path, "inputs.") {
			id := strings.TrimPrefix(path, "inputs.")
			if !strings.Contains(id, ".") && id != "" {
				found[id] = true
			}
		}
	}
	for index := range condition.Args {
		collectInputConditionRefs(&condition.Args[index], found)
	}
}

func containsJSON(items []any, value any) bool {
	target, err := json.Marshal(value)
	if err != nil {
		return false
	}
	for _, item := range items {
		raw, err := json.Marshal(item)
		if err == nil && string(raw) == string(target) {
			return true
		}
	}
	return false
}

func validSemanticVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func containsStringCore(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func compareSemanticVersions(left, right string) int {
	leftParts, rightParts := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < 3; index++ {
		leftValue, rightValue := strings.TrimLeft(leftParts[index], "0"), strings.TrimLeft(rightParts[index], "0")
		if leftValue == "" {
			leftValue = "0"
		}
		if rightValue == "" {
			rightValue = "0"
		}
		if len(leftValue) < len(rightValue) || (len(leftValue) == len(rightValue) && leftValue < rightValue) {
			return -1
		}
		if len(leftValue) > len(rightValue) || (len(leftValue) == len(rightValue) && leftValue > rightValue) {
			return 1
		}
	}
	return 0
}
