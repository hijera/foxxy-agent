//go:build miniapps

package miniapps

import (
	"encoding/json"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
)

// RewriteCommandActions rewrites successful run_command trace actions that
// match a declared command profile into calls of the profile's cmd_* tool with
// typed JSON arguments. The rewrite happens in place, before eligibility and
// candidate generation, so the rest of the pipeline — input classification,
// synthesis, evidence, replay — sees only the typed form and needs no special
// handling. Actions that do not match (different binary, template mismatch, or
// shell syntax) are left untouched; the synthesis stage decides what to do
// with them.
func RewriteCommandActions(trace *NormalizedTrace, profiles []cmdprofile.ProfileSpec) {
	if trace == nil || len(profiles) == 0 {
		return
	}
	for index := range trace.Actions {
		action := &trace.Actions[index]
		if action.Name != "run_command" || action.Status != TraceActionSucceeded {
			continue
		}
		command, ok := runCommandLine(action.Arguments)
		if !ok {
			continue
		}
		match, err := cmdprofile.MatchProfiles(command, profiles)
		if err != nil || match == nil {
			// ErrShellComplex and no-match both keep the original action; the
			// distinction matters only when the action is inside a confirmed
			// scenario, which synthesis checks explicitly.
			continue
		}
		encoded, err := json.Marshal(match.Params)
		if err != nil {
			continue
		}
		action.Name = match.Profile.ToolName()
		action.Arguments = string(encoded)
	}
}

// runCommandLine extracts the command string from run_command arguments.
// Background commands are skipped: their observable outcome lives in later
// background_* calls, not in the start call itself.
func runCommandLine(argumentsJSON string) (string, bool) {
	var arguments struct {
		Command    string `json:"command"`
		Background bool   `json:"background"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &arguments); err != nil {
		return "", false
	}
	if arguments.Background || arguments.Command == "" {
		return "", false
	}
	return arguments.Command, true
}

// commandProfileByToolName finds a declared profile by its cmd_* tool name.
func commandProfileByToolName(profiles []cmdprofile.ProfileSpec, toolName string) (cmdprofile.ProfileSpec, bool) {
	for _, profile := range profiles {
		if profile.ToolName() == toolName {
			return profile, true
		}
	}
	return cmdprofile.ProfileSpec{}, false
}
