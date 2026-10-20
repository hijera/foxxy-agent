//go:build browser

package browser

import (
	"context"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// HandleDialogTool arms the answer for the next JavaScript dialog. Dialogs block
// the page, so by default the session dismisses them (and accepts beforeunload)
// to keep actions from hanging — the page log records each one. This tool is how
// the model chooses differently: arm the answer, then perform the action that
// triggers the dialog.
func (m *Manager) HandleDialogTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name: "foxxycode_browser_handle_dialog",
			Description: "Choose how the NEXT JavaScript dialog (alert/confirm/prompt/beforeunload) in the interactive browser is answered: " +
				"call this BEFORE the click or navigation that triggers it. Unarmed dialogs are auto-dismissed (beforeunload accepted) " +
				"and reported in the page log, so nothing ever hangs.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"accept": map[string]interface{}{
						"type":        "boolean",
						"description": "true to accept (OK), false to dismiss (Cancel).",
					},
					"prompt_text": map[string]interface{}{
						"type":        "string",
						"description": "Optional text to enter when the dialog is a prompt().",
					},
				},
				"required": []interface{}{"accept"},
			},
		},
		RequiresPermission: true,
		Execute:            m.executeHandleDialog,
	}
}

type handleDialogArgs struct {
	Accept     bool   `json:"accept"`
	PromptText string `json:"prompt_text"`
}

func (m *Manager) executeHandleDialog(_ context.Context, argsJSON string, env *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[handleDialogArgs](argsJSON)
	if err != nil {
		return "", err
	}
	b, err := m.get(sessionKey(env), profileDirFor(sessionDir(env)))
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	b.armDialog(args.Accept, args.PromptText)
	verdict := "dismissed (Cancel)"
	if args.Accept {
		verdict = "accepted (OK)"
	}
	return fmt.Sprintf("armed: the next JavaScript dialog will be %s; now perform the action that triggers it", verdict), nil
}
