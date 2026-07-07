//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func newProviderModelsServer(t *testing.T, cfg *config.Config) *httptest.Server {
	t.Helper()
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), "/tmp", nil)
	srv := New(cfg, mgr, slog.Default(), "/tmp")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestProviderModelsUnknownProvider404(t *testing.T) {
	ts := newProviderModelsServer(t, &config.Config{})

	res, err := http.Get(ts.URL + "/foxxycode/providers/nope/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

func TestProviderModelsHappyPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"}]}`))
	}))
	defer upstream.Close()

	ts := newProviderModelsServer(t, &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "openai", Type: "openai", APIBase: upstream.URL, APIKey: "sk-test"},
		},
	})

	res, err := http.Get(ts.URL + "/foxxycode/providers/openai/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		OK     bool `json:"ok"`
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || len(body.Models) != 2 {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestProviderModelsProbeHappyPath(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"llama-3"},{"id":"qwen-2.5"}]}`))
	}))
	defer upstream.Close()

	// No providers configured: the probe carries credentials in the body.
	ts := newProviderModelsServer(t, &config.Config{})

	res, err := http.Post(
		ts.URL+"/foxxycode/providers/models-probe",
		"application/json",
		strings.NewReader(`{"type":"openai","api_base":"`+upstream.URL+`","api_key":"sk-probe"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		OK     bool `json:"ok"`
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || len(body.Models) != 2 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if gotAuth != "Bearer sk-probe" {
		t.Fatalf("upstream Authorization = %q, want Bearer sk-probe", gotAuth)
	}
}

func TestProviderModelsProbeUpstreamErrorReturnsOKFalse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	ts := newProviderModelsServer(t, &config.Config{})

	res, err := http.Post(
		ts.URL+"/foxxycode/providers/models-probe",
		"application/json",
		strings.NewReader(`{"type":"openai","api_base":"`+upstream.URL+`","api_key":"bad"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (graceful fallback)", res.StatusCode)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.OK {
		t.Fatal("ok = true, want false on upstream error")
	}
}

func TestProviderModelsProbeBadRequest(t *testing.T) {
	ts := newProviderModelsServer(t, &config.Config{})

	for name, payload := range map[string]string{
		"invalid json":     `{`,
		"missing type":     `{"api_base":"http://127.0.0.1:1","api_key":"x"}`,
		"unsupported type": `{"type":"grpc","api_key":"x"}`,
	} {
		res, err := http.Post(
			ts.URL+"/foxxycode/providers/models-probe",
			"application/json",
			strings.NewReader(payload),
		)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", name, res.StatusCode)
		}
	}
}

func TestProviderModelsUpstreamErrorReturnsOKFalse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	ts := newProviderModelsServer(t, &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "openai", Type: "openai", APIBase: upstream.URL, APIKey: "bad"},
		},
	})

	res, err := http.Get(ts.URL + "/foxxycode/providers/openai/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (graceful fallback)", res.StatusCode)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.OK {
		t.Fatal("ok = true, want false on upstream error")
	}
}
