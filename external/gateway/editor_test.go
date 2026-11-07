//go:build gateway || gateway.telegram

package gateway

import (
	"context"
	"testing"

	"github.com/hijera/foxxycode-agent/external/gateway/telegram"
	"github.com/hijera/foxxycode-agent/internal/acp"
)

type resumeScopeRunner struct {
	telegram.SessionRunner
	params acp.SessionListParams
}

func (r *resumeScopeRunner) HandleSessionList(_ context.Context, params acp.SessionListParams) (*acp.SessionListResult, error) {
	r.params = params
	return &acp.SessionListResult{}, nil
}

func TestEditorResumeCannotListAnotherWorkspace(t *testing.T) {
	base := &resumeScopeRunner{}
	runner := editorRunner{SessionRunner: base, cwd: "selected-project"}
	other, cursor := "another-project", "page-2"
	if _, err := runner.HandleSessionList(context.Background(), acp.SessionListParams{CWD: &other, Cursor: &cursor}); err != nil {
		t.Fatal(err)
	}
	if base.params.CWD == nil || *base.params.CWD != "selected-project" {
		t.Fatal("resume escaped the selected project")
	}
	if base.params.Cursor == nil || *base.params.Cursor != cursor {
		t.Fatal("resume lost pagination")
	}
}
