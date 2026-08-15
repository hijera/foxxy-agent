//go:build miniapps

package miniapps

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var portableIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}[a-z0-9]$`)

var supportedInputTypes = map[string]bool{
	"string": true, "text": true, "integer": true, "number": true,
	"boolean": true, "date": true, "datetime": true, "enum": true,
	"file": true, "files": true, "directory": true, "secret": true,
}

var supportedStepKinds = map[string]bool{
	"input": true, "program": true, "script": true, "command": true,
	"agent": true, "api": true, "mcp": true, "skill": true, "file": true,
	"confirm": true, "branch": true, "miniapp": true,
}

// Validate performs schema-independent semantic validation used by the editor,
// release gate, CLI, and interpreter.
func Validate(app MiniApp) ValidationReport {
	var issues []ValidationIssue
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
	if strings.TrimSpace(app.Metadata.Name) == "" {
		add("metadata.name", "is required")
	}
	if strings.TrimSpace(app.Metadata.Goal) == "" {
		add("metadata.goal", "is required")
	}
	if len(app.Workflow) == 0 {
		add("workflow", "must contain at least one step")
	}
	if app.Runtime.PersistAgentReasoning {
		add("runtime.persist_agent_reasoning", "must be false in schema v1")
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
	for i, runtime := range app.Requirements.Runtimes {
		path := fmt.Sprintf("requirements.runtimes[%d]", i)
		if !portableIDPattern.MatchString(runtime.ID) {
			add(path+".id", "must be a portable identifier")
		}
		if strings.TrimSpace(runtime.Version) == "" {
			add(path+".version", "is required")
		}
		if runtime.Provision != nil {
			if runtime.Provision.Mode != "portable" {
				add(path+".provision.mode", "must be portable")
			}
			if runtime.Provision.Interaction != "silent_private" {
				add(path+".provision.interaction", "must be silent_private")
			}
			if !strings.HasPrefix(strings.ToLower(runtime.Provision.URL), "https://") &&
				!strings.HasPrefix(strings.ToLower(runtime.Provision.URL), "http://127.0.0.1") &&
				!strings.HasPrefix(strings.ToLower(runtime.Provision.URL), "http://localhost") {
				add(path+".provision.url", "must use HTTPS (loopback HTTP is allowed for local runtimes)")
			}
			if len(runtime.Provision.SHA256) != 64 {
				add(path+".provision.sha256", "must be an exact SHA-256 digest")
			}
			if runtime.Provision.SizeBytes <= 0 {
				add(path+".provision.size_bytes", "must be positive")
			}
		}
	}
	for i, binding := range app.Requirements.ModelBindings {
		path := fmt.Sprintf("requirements.model_bindings[%d]", i)
		if !portableIDPattern.MatchString(binding.ID) {
			add(path+".id", "must be a portable identifier")
		}
		if binding.Selection != "fixed" && binding.Selection != "capability" {
			add(path+".selection", "must be fixed or capability")
		}
		if binding.Selection == "fixed" {
			if strings.TrimSpace(binding.Provider.BaseURL) == "" {
				add(path+".provider.base_url", "is required for a fixed binding")
			}
			if strings.TrimSpace(binding.Model) == "" {
				add(path+".model", "is required for a fixed binding")
			}
		}
		if binding.LocalBootstrap != nil {
			if binding.Provider.Scope != "local" {
				add(path+".local_bootstrap", "is allowed only for a local provider")
			}
			if binding.LocalBootstrap.EnsureModel != "" &&
				binding.LocalBootstrap.EnsureModel != "pull_if_missing" {
				add(path+".local_bootstrap.ensure_model", "must be pull_if_missing when set")
			}
			if binding.LocalBootstrap.StorageScope != "" &&
				binding.LocalBootstrap.StorageScope != "app_cache" {
				add(path+".local_bootstrap.storage_scope", "must be app_cache when set")
			}
		}
	}

	inputs := make(map[string]struct{}, len(app.Inputs))
	for i, input := range app.Inputs {
		path := fmt.Sprintf("inputs[%d]", i)
		if !portableIDPattern.MatchString(input.ID) {
			add(path+".id", "must be a portable identifier")
		}
		if _, exists := inputs[input.ID]; exists {
			add(path+".id", "is duplicated")
		}
		inputs[input.ID] = struct{}{}
		if !supportedInputTypes[input.Type] {
			add(path+".type", "is unsupported")
		}
		if strings.TrimSpace(input.UI.Control) == "" {
			add(path+".ui.control", "is required")
		}
		if input.Validation.Pattern != "" {
			if _, err := regexp.Compile(input.Validation.Pattern); err != nil {
				add(path+".validation.pattern", "is not a valid regular expression")
			}
		}
	}
	if cycle := inputDependencyCycle(app.Inputs); len(cycle) > 0 {
		add("inputs", "dependency graph contains a cycle: "+strings.Join(cycle, " -> "))
	}

	stepIDs := map[string]struct{}{}
	var validateSteps func([]Step, string)
	validateSteps = func(steps []Step, base string) {
		for i, step := range steps {
			path := fmt.Sprintf("%s[%d]", base, i)
			if !portableIDPattern.MatchString(step.ID) {
				add(path+".id", "must be a portable identifier")
			}
			if _, exists := stepIDs[step.ID]; exists {
				add(path+".id", "is duplicated")
			}
			stepIDs[step.ID] = struct{}{}
			if !supportedStepKinds[step.Kind] {
				add(path+".kind", "is unsupported")
			}
			switch step.Kind {
			case "program":
				if _, err := verifyProgram(programFromStep(step)); err != nil {
					add(path, err.Error())
				}
			case "command":
				if strings.TrimSpace(step.Command) == "" {
					add(path+".command", "is required")
				}
			case "script":
				if strings.TrimSpace(step.Script) == "" || strings.TrimSpace(step.ScriptLanguage) == "" {
					add(path, "script and script_language are required")
				}
			case "agent":
				if strings.TrimSpace(step.Prompt) == "" {
					add(path+".prompt", "is required")
				}
			case "api":
				if step.Request == nil || strings.TrimSpace(step.Request.URL) == "" {
					add(path+".request", "is required")
				}
			case "file":
				switch step.Operation {
				case "read", "write", "exists", "mkdir", "list", "copy", "move", "archive":
				default:
					add(path+".operation", "is unsupported")
				}
			case "branch":
				if step.If == nil {
					add(path+".if", "is required so the else branch is reachable")
				}
				validateSteps(step.Then, path+".then")
				validateSteps(step.Else, path+".else")
			case "miniapp":
				if step.AppID == "" || step.AppVersion == "" {
					add(path, "app_id and exact app_version are required")
				}
			}
		}
	}
	validateSteps(app.Workflow, "workflow")

	for i, output := range app.Outputs {
		if output.ID == "" {
			add(fmt.Sprintf("outputs[%d].id", i), "is required")
		}
	}
	if len(app.Success.Checks) == 0 {
		add("success.checks", "must contain at least one check")
	}
	if app.Success.Mode != "all" && app.Success.Mode != "any" {
		add("success.mode", "must be all or any")
	}
	for i, check := range app.Success.Checks {
		path := fmt.Sprintf("success.checks[%d]", i)
		switch check.Kind {
		case "step":
			if strings.TrimSpace(check.Step) == "" || strings.TrimSpace(check.Status) == "" {
				add(path, "step and status are required")
			}
		case "schema":
			if check.Value == nil || len(check.Schema) == 0 {
				add(path, "value and schema are required")
			}
		case "prompt":
			if check.Value == nil || strings.TrimSpace(check.Prompt) == "" {
				add(path, "value and prompt are required")
			}
			if strings.TrimSpace(check.ModelBinding) == "" ||
				!hasModelBinding(app.Requirements.ModelBindings, check.ModelBinding) {
				add(path+".model_binding", "must reference a declared model binding")
			}
		default:
			add(path+".kind", "is unsupported")
		}
	}

	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Path < issues[j].Path })
	return ValidationReport{Valid: len(issues) == 0, Issues: issues}
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
		conditionallyRequired, err := inputConditionValue(input.RequiredWhen, refs, false)
		if err != nil {
			return fmt.Errorf("input %q required_when: %w", input.ID, err)
		}
		value, ok := values[input.ID]
		if !ok || value == nil || (input.Type != "boolean" && fmt.Sprint(value) == "") {
			if input.Required || conditionallyRequired {
				return fmt.Errorf("input %q is required", input.ID)
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
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("input %q must be an array", input.ID)
			}
		}
		if len(input.Validation.Enum) > 0 && !containsJSON(input.Validation.Enum, value) {
			return fmt.Errorf("input %q is not an allowed value", input.ID)
		}
		if str, ok := value.(string); ok {
			if input.Validation.MinLength != nil && len([]rune(str)) < *input.Validation.MinLength {
				return fmt.Errorf("input %q is too short", input.ID)
			}
			if input.Validation.MaxLength != nil && len([]rune(str)) > *input.Validation.MaxLength {
				return fmt.Errorf("input %q is too long", input.ID)
			}
			if input.Validation.Pattern != "" {
				matched, _ := regexp.MatchString(input.Validation.Pattern, str)
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
			if dependency != input.ID && known[dependency] {
				graph[input.ID] = append(graph[input.ID], dependency)
			}
			if dependency == input.ID {
				return []string{input.ID, input.ID}
			}
		}
		sort.Strings(graph[input.ID])
	}
	state := make(map[string]int)
	stack := []string{}
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
				for stack[start] != dependency {
					start++
				}
				return append(append([]string{}, stack[start:]...), dependency)
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
		var ref string
		switch typed := value.(type) {
		case Ref:
			ref = typed.Ref
		case map[string]any:
			ref, _ = typed["$ref"].(string)
		}
		if strings.HasPrefix(ref, "inputs.") {
			id := strings.TrimPrefix(ref, "inputs.")
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
	target, _ := json.Marshal(value)
	for _, item := range items {
		raw, _ := json.Marshal(item)
		if string(raw) == string(target) {
			return true
		}
	}
	return false
}

func programFromStep(step Step) Program {
	return Program{
		Language: step.Language, Entry: step.Entry, Imports: step.Imports,
		Functions: step.Functions, Limits: step.Limits, OutputSchema: step.OutputSchema,
	}
}
