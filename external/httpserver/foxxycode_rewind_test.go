//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// Rewinding truncates history, which is work on the server. A page on another
// site can make a browser send a POST to a loopback server without a preflight
// only as a "simple" request - text/plain, a form encoding - so a destructive
// route must refuse those (see requireJSONRequest). The gate runs before the
// session is resolved, so the refusal does not depend on the id.
func TestSessionRewindRequiresJSONContentType(t *testing.T) {
	ts := newProviderModelsServer(t, &config.Config{})
	body := `{"userMessageIndex":0}`

	for _, contentType := range []string{"text/plain", "text/plain;charset=UTF-8", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x", ""} {
		t.Run(contentType, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/foxxycode/sessions/sess_nope/rewind", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, want 415", res.StatusCode)
			}
			var out map[string]interface{}
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			// The route answers its errors as an envelope, and the gate is one
			// of them: a client must not see a config-shaped body here.
			if msg := errorMessage(out); !strings.Contains(msg, "application/json") {
				t.Fatalf("body = %v, want an ErrorEnvelope naming application/json", out)
			}
		})
	}

	// A JSON request is not refused by the gate: it reaches the handler and is
	// answered on its own merits.
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/foxxycode/sessions/sess_nope/rewind", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode == http.StatusUnsupportedMediaType {
		t.Fatal("a JSON request was refused by the content-type gate")
	}
}
