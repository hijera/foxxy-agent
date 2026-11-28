//go:build http

package httpserver

// Unit tests for the documented error paths of the session goal route
// (goal_http.go). The happy path lives in features/session_goal_http.feature.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGoalRouteErrorPaths(t *testing.T) {
	s := &goalHTTPState{root: t.TempDir()}
	if err := s.server(); err != nil {
		t.Fatal(err)
	}
	defer s.close()

	request := func(method, body string) (int, string) {
		t.Helper()
		var rd io.Reader
		if body != "" {
			rd = bytes.NewReader([]byte(body))
		}
		req, _ := http.NewRequest(method, s.ts.URL+"/foxxycode/sessions/"+s.sid+"/goal", rd)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(raw)
	}
	code := func(body string) string {
		var out struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal([]byte(body), &out)
		return out.Error.Code
	}

	// A session without a goal reads back as one with none.
	if status, body := request(http.MethodGet, ""); status != http.StatusOK || !strings.Contains(body, `"goal":null`) {
		t.Fatalf("GET without a goal: status %d, body %s; want 200 with a null goal", status, body)
	}

	for _, body := range []string{
		`{"status":"paused","objective":"ship it"}`,
		`{}`,
		`{"status":"active"}`,
	} {
		if status, raw := request(http.MethodPatch, body); status != http.StatusBadRequest || code(raw) != "invalid_request" {
			t.Fatalf("PATCH %s: status %d, body %s; want 400 invalid_request", body, status, raw)
		}
	}

	long := `{"objective":"` + strings.Repeat("a", 4001) + `"}`
	if status, raw := request(http.MethodPatch, long); status != http.StatusBadRequest || code(raw) != "invalid_request" {
		t.Fatalf("PATCH a too-long objective: status %d, body %s; want 400 invalid_request", status, raw)
	}

	if status, raw := request(http.MethodPatch, `{"objective":`); status != http.StatusBadRequest || code(raw) != "invalid_request" {
		t.Fatalf("PATCH invalid JSON: status %d, body %s; want 400 invalid_request", status, raw)
	}

	if status, raw := request(http.MethodPatch, `{"status":"paused"}`); status != http.StatusConflict || code(raw) != "no_goal" {
		t.Fatalf("PATCH pause without a goal: status %d, body %s; want 409 no_goal", status, raw)
	}

	// Clearing is idempotent: without a goal it still answers the goal shape.
	if status, raw := request(http.MethodDelete, ""); status != http.StatusOK || !strings.Contains(raw, `"goal":null`) {
		t.Fatalf("DELETE without a goal: status %d, body %s; want 200 with a null goal", status, raw)
	}

	if status, raw := request(http.MethodGet, ""); status != http.StatusOK {
		t.Fatalf("GET after refused patches: status %d, body %s; the route changed nothing", status, raw)
	}
}
