//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func newSVNChangesState(t *testing.T) *sessionChangesState {
	t.Helper()
	s := &sessionChangesState{}
	if err := s.reset(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	if err := s.startSVNServer(); err != nil {
		if errors.Is(err, godog.ErrSkip) {
			t.Skip("svn or svnadmin is unavailable")
		}
		t.Fatal(err)
	}
	return s
}

func TestSVNSessionChangesRejectStaleRevision(t *testing.T) {
	s := newSVNChangesState(t)
	const name = "review.txt"
	if err := s.svnCommitted(name, "base\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.changedOnDisk(name, "reviewed\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.openDiff(name); err != nil {
		t.Fatal(err)
	}
	if s.one.Revision == "" {
		t.Fatal("missing review revision")
	}
	if err := s.changedOnDisk(name, "latest\n"); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(discardRequest{Paths: json.RawMessage(`["review.txt"]`), Revision: s.one.Revision})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.ts.Client().Post(s.ts.URL+"/foxxycode/sessions/"+s.sessionID+"/changes/revert",
		"application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("stale SVN revert returned %d", res.StatusCode)
	}
	if err := s.fileContains(name, "latest\n"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionChangesPrefersTheNearestCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	s := newSVNChangesState(t)
	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = s.root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize enclosing Git repository: %v\n%s", err, out)
	}
	if wc := s.srv.changesFor(context.Background(), s.workspace); wc.vcs != "svn" {
		t.Fatalf("SVN inside Git reports %q", wc.vcs)
	}
	if err := s.git("init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if wc := s.srv.changesFor(context.Background(), s.workspace); wc.vcs != "git" {
		t.Fatalf("shared checkout root reports %q", wc.vcs)
	}
}
