package session_test

import (
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/session"
)

func TestNormalizeWorkspaceRelativePathRejectsTraversal(t *testing.T) {
	if _, err := session.NormalizeWorkspaceRelativePath("../x"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := session.AbsPathUnderWorkspaceRoot(t.TempDir(), "safe"); err != nil {
		t.Fatal(err)
	}
}
