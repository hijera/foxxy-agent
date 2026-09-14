//go:build http

package httpserver

// The workspace scope of POST /foxxycode/sessions/bulk-delete. The editor
// plugins run one server per project over a session home every project shares,
// so a table scoped to one project sends that project as cwd and the server
// keeps the delete inside it.

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// storeSessionIn persists one bundle with a single user turn in workspace cwd.
func storeSessionIn(t *testing.T, mgr *session.Manager, store *session.FileStore, cwd, text string) string {
	t.Helper()
	res, err := mgr.HandleSessionNew(t.Context(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	st := mgr.SessionByID(res.SessionID)
	if st == nil {
		t.Fatalf("session %q not registered", res.SessionID)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: text})
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	return res.SessionID
}

// Scope "all" with cwd removes the sessions of that folder and of the folders
// beneath it, and nothing another project stored in the same home.
func TestBulkDeleteAllStaysInsideTheRequestedWorkspace(t *testing.T) {
	srv, mgr, store := bulkDeleteServer(t)
	project := t.TempDir()
	nested := filepath.Join(project, "module")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	here := storeSessionIn(t, mgr, store, project, "in the project")
	below := storeSessionIn(t, mgr, store, nested, "in a folder of the project")
	other := storeSessionIn(t, mgr, store, t.TempDir(), "in another project")

	code, body := postBulkDelete(t, srv, map[string]interface{}{"scope": "all", "cwd": project})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	for _, id := range []string{here, below} {
		if store.HasPersistedSnapshot(id) {
			t.Fatalf("session %q of the project survived scope=all", id)
		}
	}
	if !store.HasPersistedSnapshot(other) {
		t.Fatal("scope=all with cwd removed a session of another project")
	}
}

// An ids list with cwd is checked whole before anything goes: one stored
// session outside the workspace refuses the request, and the in-scope id listed
// beside it is not removed either.
func TestBulkDeleteRefusesAnIDOutsideTheRequestedWorkspace(t *testing.T) {
	srv, mgr, store := bulkDeleteServer(t)
	project := t.TempDir()
	here := storeSessionIn(t, mgr, store, project, "in the project")
	other := storeSessionIn(t, mgr, store, t.TempDir(), "in another project")

	code, body := postBulkDelete(t, srv, map[string]interface{}{
		"ids": []string{here, other},
		"cwd": project,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	for _, id := range []string{here, other} {
		if !store.HasPersistedSnapshot(id) {
			t.Fatalf("session %q was removed by a refused request", id)
		}
	}

	// The same list without the foreign id goes through, and an id that has no
	// bundle any more is still admitted: removing it removes nothing.
	code, body = postBulkDelete(t, srv, map[string]interface{}{
		"ids": []string{here, "sess_deadbeefdeadbeefdeadbeef"},
		"cwd": project,
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	if deleted, _ := body["deleted"].([]interface{}); len(deleted) != 2 {
		t.Fatalf("deleted = %v, want both ids", body["deleted"])
	}
	if store.HasPersistedSnapshot(here) {
		t.Fatal("the in-scope session survived")
	}
	if !store.HasPersistedSnapshot(other) {
		t.Fatal("the session of another project was removed")
	}
}

// A stored bundle whose session.json does not parse cannot be shown to sit in
// the workspace, so a scoped request refuses it rather than guessing.
func TestBulkDeleteRefusesAnUnreadableSessionUnderAScope(t *testing.T) {
	srv, mgr, store := bulkDeleteServer(t)
	project := t.TempDir()
	id := storeSessionIn(t, mgr, store, project, "about to be damaged")
	if err := os.WriteFile(filepath.Join(store.SessionPath(id), "session.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, body := postBulkDelete(t, srv, map[string]interface{}{"ids": []string{id}, "cwd": project})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	if !store.HasPersistedSnapshot(id) {
		t.Fatal("the unreadable session was removed by a refused request")
	}
}

// A relative cwd would be resolved against the server's own directory, which
// is not a folder the caller named.
func TestBulkDeleteRefusesARelativeWorkspace(t *testing.T) {
	srv, mgr, store := bulkDeleteServer(t)
	id := storeSessionIn(t, mgr, store, t.TempDir(), "kept")

	for _, payload := range []map[string]interface{}{
		{"ids": []string{id}, "cwd": "relative/project"},
		{"scope": "all", "cwd": "."},
	} {
		code, body := postBulkDelete(t, srv, payload)
		if code != http.StatusBadRequest {
			t.Fatalf("payload %v: status = %d, want 400: %v", payload, code, body)
		}
	}
	if !store.HasPersistedSnapshot(id) {
		t.Fatal("a refused request removed a session")
	}
}
