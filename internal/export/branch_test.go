package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// An exported transcript says which branch the work happened on. A Subversion
// working copy has a branch as much as a git repository does, and a folder can
// be both, so the two are separate fields rather than one that guesses.
func TestExportNamesTheSubversionBranch(t *testing.T) {
	t.Parallel()
	doc := BuildExportDocument(ExportInput{
		SessionID: "sess_branch",
		CWD:       "/work/project",
		SVNBranch: "branches/feature-x",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "question"},
			{Role: llm.RoleAssistant, Content: "answer"},
		},
	})

	md, err := RenderExport(doc, ExportFormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "- **SVN branch**: `branches/feature-x`") {
		t.Errorf("markdown header does not name the SVN branch:\n%s", md)
	}
	if strings.Contains(string(md), "Git branch") {
		t.Errorf("markdown header shows an empty git branch:\n%s", md)
	}

	if head := documentHeaderMarkdown(doc); !strings.Contains(head, "- **SVN branch:** branches/feature-x") {
		t.Errorf("the HTML/PDF/DOCX header does not name the SVN branch:\n%s", head)
	}

	raw, err := RenderExport(doc, ExportFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Session map[string]any `json:"session"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if got := parsed.Session["svn_branch"]; got != "branches/feature-x" {
		t.Errorf("session.svn_branch = %v, want branches/feature-x", got)
	}
	if _, ok := parsed.Session["git_branch"]; ok {
		t.Errorf("session.git_branch is present for a folder that is not a git repository: %v", parsed.Session)
	}
}

func TestExportLeavesOutTheSubversionBranchOfAPlainFolder(t *testing.T) {
	t.Parallel()
	doc := BuildExportDocument(ExportInput{
		SessionID: "sess_plain",
		GitBranch: "main",
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: "question"}},
	})
	raw, err := RenderExport(doc, ExportFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "svn_branch") {
		t.Errorf("svn_branch is present without a working copy:\n%s", raw)
	}
	md, err := RenderExport(doc, ExportFormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(md), "SVN branch") {
		t.Errorf("markdown header shows an empty SVN branch:\n%s", md)
	}
}
