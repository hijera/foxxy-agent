package cmdprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// Tool builds the registry tool for one validated profile. Permission variant
// Б: an ask-profile carries RequiresPermission and prompts in chat like any
// permission-bearing tool; an allow-profile runs without a prompt and is the
// only kind Mini Apps accept for unattended runs.
func Tool(spec cmdprofile.ProfileSpec) *tooling.Tool {
	spec = spec.Clone()
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        spec.ToolName(),
			Description: toolDescription(spec),
			InputSchema: inputSchema(spec),
		},
		RequiresPermission: spec.ResolvedPermission() == cmdprofile.PermissionAsk,
		Execute: func(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
			return execute(ctx, spec, argsJSON, env)
		},
	}
}

func toolDescription(spec cmdprofile.ProfileSpec) string {
	description := strings.TrimSpace(spec.Description)
	if description == "" {
		description = "Run the " + spec.Binary + " command with fixed, operator-approved arguments."
	}
	return description + " Executes `" + spec.Binary + " " + strings.Join(spec.Template, " ") +
		"` with the provided parameters substituted; the command never passes through a shell."
}

// inputSchema renders the typed params as a JSON Schema for the model.
func inputSchema(spec cmdprofile.ProfileSpec) map[string]interface{} {
	properties := map[string]interface{}{}
	var required []string
	for _, param := range spec.Params {
		property := map[string]interface{}{}
		if description := strings.TrimSpace(param.Description); description != "" {
			property["description"] = description
		}
		switch param.Type {
		case cmdprofile.ParamInt:
			property["type"] = "integer"
			if param.Min != nil {
				property["minimum"] = *param.Min
			}
			if param.Max != nil {
				property["maximum"] = *param.Max
			}
		case cmdprofile.ParamFlag:
			property["type"] = "boolean"
		case cmdprofile.ParamEnum:
			property["type"] = "string"
			values := make([]interface{}, 0, len(param.Enum))
			for _, value := range param.Enum {
				values = append(values, value)
			}
			property["enum"] = values
		case cmdprofile.ParamFile:
			property["type"] = "string"
			if property["description"] == nil {
				property["description"] = "Filesystem path, resolved against the working directory."
			}
		default: // string with a pattern
			property["type"] = "string"
			property["pattern"] = param.Pattern
		}
		properties[param.Name] = property
		if param.Type != cmdprofile.ParamFlag {
			required = append(required, param.Name)
		}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// execute decodes the model's arguments into the string map the core expects
// and runs the profile confined to the session working directory.
func execute(ctx context.Context, spec cmdprofile.ProfileSpec, argsJSON string, env *tooling.Env) (string, error) {
	params, err := decodeParams(spec, argsJSON)
	if err != nil {
		return "", err
	}
	cwd := ""
	if env != nil {
		cwd = env.CWD
	}
	out, err := cmdprofile.Run(ctx, spec, params, cmdprofile.ExecOptions{CWD: cwd, ForbiddenRoot: cwd})
	if err != nil {
		var missing *cmdprofile.BinaryNotFoundError
		if errors.As(err, &missing) {
			// Actionable, like the svn family's message: name the fix, not
			// just the failure. In chat the agent can offer to run the
			// install command via run_command with the usual prompt.
			return "", fmt.Errorf("%s%s", missing.Error(), installHint(spec))
		}
		return "", err
	}
	return out, nil
}

func installHint(spec cmdprofile.ProfileSpec) string {
	managers := cmdprofile.DetectManagers(spec)
	if len(managers) == 0 {
		return "; install it and ensure it is on PATH, or set an absolute path in commands[name=" + spec.Name + "].binary"
	}
	var hints []string
	for _, manager := range managers {
		hints = append(hints, strings.Join(manager.Argv, " "))
	}
	return "; install it with: " + strings.Join(hints, "  |  ")
}

// decodeParams turns the model's JSON arguments into the validated string map
// BuildArgv consumes. Booleans and numbers arrive as JSON types.
func decodeParams(spec cmdprofile.ProfileSpec, argsJSON string) (map[string]string, error) {
	raw, err := tooling.ParseArgs[map[string]json.RawMessage](argsJSON)
	if err != nil {
		return nil, err
	}
	params := map[string]string{}
	for _, param := range spec.Params {
		value, present := raw[param.Name]
		if !present {
			continue
		}
		switch param.Type {
		case cmdprofile.ParamFlag:
			var flag bool
			if err := json.Unmarshal(value, &flag); err != nil {
				return nil, fmt.Errorf("param %q must be a boolean", param.Name)
			}
			params[param.Name] = strconv.FormatBool(flag)
		case cmdprofile.ParamInt:
			var number int
			if err := json.Unmarshal(value, &number); err != nil {
				return nil, fmt.Errorf("param %q must be an integer", param.Name)
			}
			params[param.Name] = strconv.Itoa(number)
		default:
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return nil, fmt.Errorf("param %q must be a string", param.Name)
			}
			params[param.Name] = text
		}
	}
	return params, nil
}
