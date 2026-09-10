//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/gitws"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type changesEnv struct {
	srv        *Server
	id         string
	cwd        string
	sessionDir string
}

func newChangesEnv(t *testing.T) *changesEnv {
	t.Helper()
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "p1", Type: "openai", APIKey: "k"}},
		Models:    []config.ModelEntry{{Model: "p1/gpt-4o"}},
		Agent:     config.Agent{Model: "p1/gpt-4o"},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	root := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), t.TempDir(), store)
	srv := New(cfg, mgr, slog.Default(), t.TempDir())

	cwd := t.TempDir()
	newRes, err := mgr.HandleSessionNew(t.Context(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	return &changesEnv{
		srv:        srv,
		id:         newRes.SessionID,
		cwd:        cwd,
		sessionDir: filepath.Join(root, "sessions", newRes.SessionID),
	}
}

func (e *changesEnv) storeTurn(t *testing.T, turn int, changes ...session.WorkspaceChange) {
	t.Helper()
	if err := session.StoreWorkspaceDiff(e.sessionDir, turn, &session.WorkspaceDiff{Changes: changes}); err != nil {
		t.Fatalf("store turn %d: %v", turn, err)
	}
}

func (e *changesEnv) do(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.SetPathValue("id", e.id)
	rec := httptest.NewRecorder()
	e.srv.mux.ServeHTTP(rec, req)
	return rec
}

func wsFile(content string) *session.WorkspaceFile {
	return &session.WorkspaceFile{Content: []byte(content), Mode: 0o644}
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

func TestSessionChangesEmpty(t *testing.T) {
	e := newChangesEnv(t)
	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	files, _ := body["files"].([]interface{})
	if len(files) != 0 {
		t.Fatalf("want no files, got %v", files)
	}
	totals, _ := body["totals"].(map[string]interface{})
	if totals["files"].(float64) != 0 {
		t.Fatalf("want zero totals, got %v", totals)
	}
}

func TestSessionChangesListStats(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1,
		session.WorkspaceChange{Path: "a.txt", Before: wsFile("one\ntwo\n"), After: wsFile("one\nTWO\nthree\n")},
		session.WorkspaceChange{Path: "b.txt", After: wsFile("x\ny\n")},
	)

	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	files := body["files"].([]interface{})
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	first := files[0].(map[string]interface{})
	if first["path"] != "a.txt" || first["status"] != "modified" {
		t.Fatalf("unexpected first file: %v", first)
	}
	if first["additions"].(float64) != 2 || first["deletions"].(float64) != 1 {
		t.Fatalf("a.txt stats wrong: %v", first)
	}
	if _, ok := first["patch"]; ok {
		t.Fatalf("list must not carry patches by default: %v", first)
	}
	second := files[1].(map[string]interface{})
	if second["status"] != "added" || second["additions"].(float64) != 2 {
		t.Fatalf("b.txt wrong: %v", second)
	}
	totals := body["totals"].(map[string]interface{})
	if totals["files"].(float64) != 2 || totals["additions"].(float64) != 4 || totals["deletions"].(float64) != 1 {
		t.Fatalf("totals wrong: %v", totals)
	}
}

func TestSessionChangesIncludePatchAndContent(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "a.txt", Before: wsFile("one\n"), After: wsFile("two\n")})

	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?include=patch,content")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	file := decodeJSON(t, rec)["files"].([]interface{})[0].(map[string]interface{})
	patch, _ := file["patch"].(string)
	if !strings.Contains(patch, "-one") || !strings.Contains(patch, "+two") {
		t.Fatalf("patch missing changes: %q", patch)
	}
	if file["before"] != "one\n" || file["after"] != "two\n" {
		t.Fatalf("content sides wrong: %v", file)
	}
}

func TestSessionChangesFileDetail(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "dir/a.txt", Before: wsFile("one\n"), After: wsFile("two\n")})

	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes/file?path=dir/a.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if body["path"] != "dir/a.txt" || body["status"] != "modified" {
		t.Fatalf("unexpected body: %v", body)
	}
	if !strings.Contains(body["patch"].(string), "+++ b/dir/a.txt") {
		t.Fatalf("patch header missing: %v", body["patch"])
	}
}

// The path comes from the request, so it is matched against the change set
// rather than resolved on disk: a traversal attempt names no changed file.
func TestSessionChangesFileRejectsUnknownPath(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "a.txt", After: wsFile("x\n")})

	for _, p := range []string{"../../etc/passwd", "b.txt", ""} {
		rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes/file?path="+p)
		if rec.Code == http.StatusOK {
			t.Fatalf("path %q was served: %s", p, rec.Body.String())
		}
	}
}

// A binary file has no line diff; it must still be listed so the user knows it
// changed, but with no patch and no line counts.
func TestSessionChangesMarksBinary(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{
		Path:  "logo.png",
		After: &session.WorkspaceFile{Content: []byte{0x89, 'P', 'N', 'G', 0x00, 0x01}, Mode: 0o644},
	})

	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?include=patch")
	file := decodeJSON(t, rec)["files"].([]interface{})[0].(map[string]interface{})
	if file["binary"] != true {
		t.Fatalf("want binary, got %v", file)
	}
	if file["additions"].(float64) != 0 || file["patch"] != "" {
		t.Fatalf("binary file must carry no line diff: %v", file)
	}
}

func TestSessionChangesRevertRestoresFiles(t *testing.T) {
	e := newChangesEnv(t)
	edited := filepath.Join(e.cwd, "a.txt")
	created := filepath.Join(e.cwd, "new.txt")
	if err := os.WriteFile(edited, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created, []byte("fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.storeTurn(t, 1,
		session.WorkspaceChange{Path: "a.txt", Before: wsFile("old\n"), After: wsFile("new\n")},
		session.WorkspaceChange{Path: "new.txt", After: wsFile("fresh\n")},
	)

	rec := e.do(t, http.MethodPost, "/foxxycode/sessions/"+e.id+"/changes/revert")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got, err := os.ReadFile(edited)
	if err != nil || string(got) != "old\n" {
		t.Fatalf("a.txt not restored: %q %v", got, err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("new.txt should have been removed, got %v", err)
	}
}

// The SPA card in an editor embed hands the review off to the plugin, which
// listens on the shared IDE event stream.
func TestSessionChangesOpenInIDEBroadcasts(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "a.txt", After: wsFile("x\n")})

	ch := ideEvents.subscribe()
	defer ideEvents.unsubscribe(ch)

	rec := e.do(t, http.MethodPost, "/foxxycode/sessions/"+e.id+"/changes/open-in-ide")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if decodeJSON(t, rec)["delivered"] != true {
		t.Fatalf("want delivered true with a subscriber: %s", rec.Body.String())
	}
	select {
	case ev := <-ch:
		if ev.Type != "open_changes" || ev.SessionID != e.id {
			t.Fatalf("unexpected event: %+v", ev)
		}
		if ev.Path != "" {
			t.Fatalf("a summary review names no file, got %q", ev.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no open_changes event")
	}
}

// Clicking one file row on the card must open the viewer on that file, so the
// hand-off event carries the clicked path for the plugin to select.
func TestSessionChangesOpenInIDECarriesFocusPath(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1,
		session.WorkspaceChange{Path: "a.txt", After: wsFile("x\n")},
		session.WorkspaceChange{Path: "dir/b.txt", After: wsFile("y\n")},
	)

	ch := ideEvents.subscribe()
	defer ideEvents.unsubscribe(ch)

	req := httptest.NewRequest(http.MethodPost,
		"/foxxycode/sessions/"+e.id+"/changes/open-in-ide",
		strings.NewReader(`{"path":"dir/b.txt"}`))
	req.SetPathValue("id", e.id)
	rec := httptest.NewRecorder()
	e.srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case ev := <-ch:
		if ev.Type != "open_changes" || ev.Path != "dir/b.txt" {
			t.Fatalf("event must carry the clicked path: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no open_changes event")
	}
}

// The viewer's scope switcher asks the same route for a narrower change set;
// "turn" must describe only the newest turn.
func TestSessionChangesTurnScope(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "old.txt", After: wsFile("one\n")})
	e.storeTurn(t, 2, session.WorkspaceChange{Path: "new.txt", After: wsFile("two\nthree\n")})

	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=turn")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if body["scope"] != "turn" {
		t.Fatalf("scope not echoed: %v", body["scope"])
	}
	files := body["files"].([]interface{})
	if len(files) != 1 {
		t.Fatalf("want only the last turn's file, got %v", files)
	}
	if files[0].(map[string]interface{})["path"] != "new.txt" {
		t.Fatalf("wrong file: %v", files[0])
	}
	totals := body["totals"].(map[string]interface{})
	if totals["additions"].(float64) != 2 {
		t.Fatalf("totals must cover the turn only: %v", totals)
	}
}

// Default scope stays the whole session so the card and the viewer agree.
func TestSessionChangesDefaultsToSessionScope(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "old.txt", After: wsFile("one\n")})
	e.storeTurn(t, 2, session.WorkspaceChange{Path: "new.txt", After: wsFile("two\n")})

	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes")
	body := decodeJSON(t, rec)
	if body["scope"] != "session" {
		t.Fatalf("want session scope, got %v", body["scope"])
	}
	if len(body["files"].([]interface{})) != 2 {
		t.Fatalf("want both turns' files: %v", body["files"])
	}
}

func TestSessionChangesRejectsUnknownScope(t *testing.T) {
	e := newChangesEnv(t)
	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=everything")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for an unknown scope, got %d: %s", rec.Code, rec.Body.String())
	}
}

// The detail route has to honour the same scope, or expanding a file in the
// viewer would show the session's diff while the list showed the turn's.
func TestSessionChangeFileHonoursScope(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "old.txt", After: wsFile("one\n")})
	e.storeTurn(t, 2, session.WorkspaceChange{Path: "new.txt", After: wsFile("two\n")})

	rec := e.do(t, http.MethodGet,
		"/foxxycode/sessions/"+e.id+"/changes/file?scope=turn&path=new.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	// old.txt belongs to an earlier turn, so it is not in this scope.
	rec = e.do(t, http.MethodGet,
		"/foxxycode/sessions/"+e.id+"/changes/file?scope=turn&path=old.txt")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 outside the scope, got %d", rec.Code)
	}
}

// On a workspace that is not a repository the scope answers empty and says git
// is unavailable, so the viewer disables the option instead of erroring.
func TestSessionChangesUncommittedOutsideARepo(t *testing.T) {
	e := newChangesEnv(t)
	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=uncommitted")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if body["vcsAvailable"] != false {
		t.Fatalf("want vcsAvailable false outside a repo: %v", body)
	}
	if len(body["files"].([]interface{})) != 0 {
		t.Fatalf("want no files: %v", body["files"])
	}
}

func gitInRepo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// The uncommitted scope reads the working copy rather than the session's turn
// diffs, so it reports edits the agent never made - and maps git's statuses onto
// the same added/modified/deleted vocabulary every other scope uses.
func TestSessionChangesUncommittedScopeReadsWorkingCopy(t *testing.T) {
	if !gitws.GitAvailable() {
		t.Skip("git binary not available")
	}
	e := newChangesEnv(t)
	if err := os.WriteFile(filepath.Join(e.cwd, "tracked.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInRepo(t, e.cwd, "init", "-b", "main")
	gitInRepo(t, e.cwd, "add", "-A")
	gitInRepo(t, e.cwd, "-c", "user.email=foxxycode@test", "-c", "user.name=foxxycode",
		"commit", "-m", "init")

	// One tracked edit the session knows nothing about, one ignored-by-nobody
	// new file that must be counted but not listed.
	if err := os.WriteFile(filepath.Join(e.cwd, "tracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.cwd, "loose.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=uncommitted")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if body["vcsAvailable"] != true {
		t.Fatalf("want vcsAvailable true in a repo: %v", body)
	}
	if body["untracked"].(float64) != 1 {
		t.Fatalf("want 1 untracked, got %v", body["untracked"])
	}
	files := body["files"].([]interface{})
	if len(files) != 1 {
		t.Fatalf("untracked files must not be listed: %v", files)
	}
	file := files[0].(map[string]interface{})
	if file["path"] != "tracked.txt" || file["status"] != "modified" {
		t.Fatalf("unexpected file: %v", file)
	}
	if file["additions"].(float64) != 1 || file["deletions"].(float64) != 1 {
		t.Fatalf("stats wrong: %v", file)
	}
}

// The list reports OS-shaped paths, so on Windows they carry backslashes. A
// caller that sends the same file with forward slashes means the same file, and
// the match is on the change set either way - it never touches the filesystem.
func TestSessionChangeFileAcceptsEitherSeparator(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{
		Path:   filepath.Join("dir", "sub", "a.txt"),
		Before: wsFile("one\n"),
		After:  wsFile("two\n"),
	})

	for _, p := range []string{"dir/sub/a.txt", "dir%5Csub%5Ca.txt"} {
		rec := e.do(t, http.MethodGet,
			"/foxxycode/sessions/"+e.id+"/changes/file?path="+p)
		if rec.Code != http.StatusOK {
			t.Fatalf("path %q returned %d: %s", p, rec.Code, rec.Body.String())
		}
	}

	// A path outside the change set is still refused, whichever separator it uses.
	rec := e.do(t, http.MethodGet,
		"/foxxycode/sessions/"+e.id+"/changes/file?path=dir/sub/other.txt")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path must be 404, got %d", rec.Code)
	}
}

// The all-files scope answers "what is in this working copy that HEAD has not",
// which is the tracked edits plus the brand-new files the git scope leaves out.
func TestSessionChangesAllScopeIncludesUntracked(t *testing.T) {
	if !gitws.GitAvailable() {
		t.Skip("git binary not available")
	}
	e := newChangesEnv(t)
	if err := os.WriteFile(filepath.Join(e.cwd, "tracked.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInRepo(t, e.cwd, "init", "-b", "main")
	gitInRepo(t, e.cwd, "add", "-A")
	gitInRepo(t, e.cwd, "-c", "user.email=foxxycode@test", "-c", "user.name=foxxycode",
		"commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(e.cwd, "tracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.cwd, "fresh.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=all")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if body["scope"] != "all" {
		t.Fatalf("scope not echoed: %v", body["scope"])
	}
	if body["vcsAvailable"] != true {
		t.Fatalf("want vcsAvailable true in a repo: %v", body)
	}
	if body["untracked"].(float64) != 0 {
		t.Fatalf("nothing was skipped, so untracked must be 0: %v", body["untracked"])
	}
	byPath := map[string]map[string]interface{}{}
	for _, f := range body["files"].([]interface{}) {
		row := f.(map[string]interface{})
		byPath[row["path"].(string)] = row
	}
	if len(byPath) != 2 {
		t.Fatalf("want tracked and untracked, got %v", byPath)
	}
	if row := byPath["fresh.txt"]; row["status"] != "added" || row["additions"].(float64) != 2 {
		t.Fatalf("fresh.txt wrong: %v", row)
	}
	// The tracked-only scope still leaves the new file out; that is the whole
	// reason the two scopes both exist.
	rec = e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=uncommitted")
	for _, f := range decodeJSON(t, rec)["files"].([]interface{}) {
		if f.(map[string]interface{})["path"] == "fresh.txt" {
			t.Fatal("the uncommitted scope must stay tracked-only")
		}
	}
}

// Expanding an untracked file must read that file and no others.
func TestSessionChangeFileServesAnUntrackedFile(t *testing.T) {
	if !gitws.GitAvailable() {
		t.Skip("git binary not available")
	}
	e := newChangesEnv(t)
	if err := os.WriteFile(filepath.Join(e.cwd, "kept.txt"), []byte("k\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInRepo(t, e.cwd, "init", "-b", "main")
	gitInRepo(t, e.cwd, "add", "-A")
	gitInRepo(t, e.cwd, "-c", "user.email=foxxycode@test", "-c", "user.name=foxxycode",
		"commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(e.cwd, "fresh.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := e.do(t, http.MethodGet,
		"/foxxycode/sessions/"+e.id+"/changes/file?scope=all&path=fresh.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if body["status"] != "added" || !strings.Contains(body["patch"].(string), "+hello") {
		t.Fatalf("unexpected body: %v", body)
	}
	// A file git does not report is not reachable through the scope either.
	rec = e.do(t, http.MethodGet,
		"/foxxycode/sessions/"+e.id+"/changes/file?scope=all&path=kept.txt")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("an unchanged file must be 404, got %d", rec.Code)
	}
}

// svnInRepo runs the svn client in a working copy, failing the test on error.
func svnInRepo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("svn", append([]string{"--non-interactive"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("svn %v: %v\n%s", args, err, out)
	}
}

// A Subversion workspace answers the working-copy scopes the same way a git one
// does: the viewer asks for a scope, not for a VCS.
func TestSessionChangesWorkingCopyScopesUnderSVN(t *testing.T) {
	for _, bin := range []string{"svn", "svnadmin"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	e := newChangesEnv(t)
	repo := filepath.Join(t.TempDir(), "repo")
	if out, err := exec.Command("svnadmin", "create", repo).CombinedOutput(); err != nil {
		t.Fatalf("svnadmin create: %v\n%s", err, out)
	}
	url := "file:///" + strings.ReplaceAll(filepath.ToSlash(repo), " ", "%20")
	if out, err := exec.Command("svn", "--non-interactive", "checkout", url, e.cwd).CombinedOutput(); err != nil {
		t.Fatalf("svn checkout: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(e.cwd, "tracked.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svnInRepo(t, e.cwd, "add", "tracked.txt")
	svnInRepo(t, e.cwd, "commit", "-m", "init")

	if err := os.WriteFile(filepath.Join(e.cwd, "tracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.cwd, "fresh.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Tracked-only: the edit shows, the new file is counted but not listed.
	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=uncommitted")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if body["vcsAvailable"] != true {
		t.Fatalf("an svn working copy must report a usable VCS: %v", body)
	}
	files := body["files"].([]interface{})
	if len(files) != 1 || files[0].(map[string]interface{})["path"] != "tracked.txt" {
		t.Fatalf("want only tracked.txt, got %v", files)
	}
	if body["untracked"].(float64) != 1 {
		t.Fatalf("want 1 unversioned counted, got %v", body["untracked"])
	}

	// All files: the new one joins in.
	rec = e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=all")
	byPath := map[string]map[string]interface{}{}
	for _, f := range decodeJSON(t, rec)["files"].([]interface{}) {
		row := f.(map[string]interface{})
		byPath[row["path"].(string)] = row
	}
	if len(byPath) != 2 {
		t.Fatalf("want both files, got %v", byPath)
	}
	if row := byPath["fresh.txt"]; row["status"] != "added" || row["additions"].(float64) != 2 {
		t.Fatalf("fresh.txt wrong: %v", row)
	}

	// And one file resolves on its own, unversioned included.
	rec = e.do(t, http.MethodGet,
		"/foxxycode/sessions/"+e.id+"/changes/file?scope=all&path=fresh.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(decodeJSON(t, rec)["patch"].(string), "+a") {
		t.Fatalf("unexpected patch: %v", decodeJSON(t, rec)["patch"])
	}
}
