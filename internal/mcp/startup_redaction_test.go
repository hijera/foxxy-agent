package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/config"
)

type startupLogBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *startupLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *startupLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func startupCredentialFailure(t *testing.T) config.MCPServerConfig {
	t.Helper()
	t.Setenv("FOXXY_MCP_STARTUP_TEST_TOKEN", "MixedCase-Startup-Test-Secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		message := fmt.Sprintf("HTTP 401 url=http://%s%s TOKEN=%s short=%s digit=%s",
			r.Host, r.URL.RequestURI(), strings.ToUpper(r.Header.Get("Authorization")),
			strings.ToUpper(r.Header.Get("X-Short")), r.Header.Get("X-Digit"))
		if r.Method == http.MethodGet {
			http.Error(w, message, http.StatusUnauthorized)
			return
		}
		var request struct {
			ID any `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID,
			"error": map[string]any{"code": -32000, "message": message},
		})
	}))
	t.Cleanup(server.Close)
	return config.MCPServerConfig{
		Name: "startup-redaction", URL: server.URL + "/mcp?token=${FOXXY_MCP_STARTUP_TEST_TOKEN}",
		Headers: []config.HTTPHeaderConfig{
			{Name: "Authorization", Value: "Bearer ${FOXXY_MCP_STARTUP_TEST_TOKEN}"},
			{Name: "X-Short", Value: "aBc123"},
			{Name: "X-Digit", Value: "1"},
		},
	}
}

func checkStartupCredentials(message string) error {
	for _, secret := range []string{"mixedcase-startup-test-secret", "abc123", "digit=1"} {
		if strings.Contains(strings.ToLower(message), secret) {
			return fmt.Errorf("startup error exposed a configured credential: %s", message)
		}
	}
	if !strings.Contains(message, config.RedactedValue) {
		return fmt.Errorf("startup error has no redacted placeholder: %s", message)
	}
	return nil
}

func TestMCPStartupRedactionFeature(t *testing.T) {
	var srv config.MCPServerConfig
	var output startupLogBuffer
	var warning string
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^an MCP server that echoes configured credentials in its startup error$`, func() {
				srv = startupCredentialFailure(t)
			})
			sc.Step(`^the server pool keeps that global MCP server$`, func() error {
				pool := NewPool(slog.New(slog.NewJSONHandler(&output, nil)))
				t.Cleanup(pool.Close)
				pool.Keep([]ManagedServer{globalServer(srv)})
				if !eventually(t, 5*time.Second, func() bool {
					return strings.Contains(output.String(), "MCP server did not start")
				}) {
					return fmt.Errorf("missing backend startup warning: %s", output.String())
				}
				var record struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal([]byte(strings.TrimSpace(output.String())), &record); err != nil {
					return err
				}
				warning = record.Error
				return nil
			})
			sc.Step(`^the backend startup warning masks those credentials$`, func() error {
				return checkStartupCredentials(warning)
			})
			sc.Step(`^the startup warning keeps the HTTP status readable$`, func() error {
				if !strings.Contains(warning, "HTTP 401") {
					return fmt.Errorf("startup warning lost the HTTP status: %s", warning)
				}
				return nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/mcp_startup_redaction.feature"}, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("MCP startup redaction scenario failed")
	}
}

func TestConnectRedactsCredentialFailures(t *testing.T) {
	for _, transport := range []string{"http", "sse"} {
		t.Run(transport, func(t *testing.T) {
			srv := startupCredentialFailure(t)
			srv.Type = transport
			client, err := Connect(testCtx(t), srv, t.TempDir(), slog.Default())
			if client != nil {
				_ = client.Close()
			}
			if err == nil {
				t.Fatal("the server's refusal unexpectedly connected")
			}
			if err := checkStartupCredentials(err.Error()); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(err.Error(), "401") {
				t.Fatalf("startup error lost HTTP status: %v", err)
			}
		})
	}
}

func TestRedactedStartupErrorPreservesCause(t *testing.T) {
	const secret = "Wrapped-Startup-Test-Secret"
	srv := config.MCPServerConfig{Env: []config.EnvVarConfig{{Name: "TOKEN", Value: secret}}}
	cause := &url.Error{Op: "Post", URL: "https://example.invalid/?token=" + secret, Err: context.DeadlineExceeded}
	got := redactError(srv, t.TempDir(), cause)
	if strings.Contains(got.Error(), secret) || !strings.Contains(got.Error(), config.RedactedValue) {
		t.Fatalf("unsafe startup error: %v", got)
	}
	if !errors.Is(got, context.DeadlineExceeded) {
		t.Fatal("redaction lost the deadline cause used by retry classification")
	}
	var transport *url.Error
	if !errors.As(got, &transport) || transport != cause {
		t.Fatal("redaction lost the transport error type")
	}
	if redactError(srv, "", nil) != nil {
		t.Fatal("redaction turned a successful connection into an error")
	}
	plain := errors.New("connection refused")
	if redactError(srv, "", plain) != plain {
		t.Fatal("redaction changed an error with no credentials")
	}
}

func TestConnectRedactsSuccessfulFallbackDiagnostic(t *testing.T) {
	const secret = "MixedCase-Fallback-Test-Secret"
	legacy := fakeLegacySSEMux(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sse" {
			http.Error(w, "HTTP 400 TOKEN="+strings.ToUpper(r.Header.Get("Authorization")), http.StatusBadRequest)
			return
		}
		legacy.ServeHTTP(w, r)
	}))
	defer server.Close()
	var output startupLogBuffer
	srv := config.MCPServerConfig{Name: "fallback", URL: server.URL + "/sse",
		Headers: []config.HTTPHeaderConfig{{Name: "Authorization", Value: secret}}}
	client, err := Connect(testCtx(t), srv, t.TempDir(), slog.New(slog.NewJSONHandler(&output, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if len(client.Tools()) != 1 {
		t.Fatal("the healthy SSE fallback lost its tools")
	}
	text := output.String()
	if !strings.Contains(text, "connected via legacy SSE") || !strings.Contains(text, config.RedactedValue) ||
		strings.Contains(strings.ToLower(text), strings.ToLower(secret)) {
		t.Fatalf("fallback diagnostic was not safely retained: %s", text)
	}
}

func TestConnectionLoggerKeepsStructuredFields(t *testing.T) {
	const secret = "Structured-Test-Secret"
	var output startupLogBuffer
	handler := redactingHandler{Handler: slog.NewJSONHandler(&output, nil),
		server: config.MCPServerConfig{Env: []config.EnvVarConfig{{Name: "TOKEN", Value: secret}}}}
	log := slog.New(handler).WithGroup("transport").With("token", secret)
	log.Warn("HTTP 401 "+secret, "attempt", 2,
		slog.Group("details", "error", fmt.Errorf("initialize refused %s", secret)))
	var record struct {
		Message   string `json:"msg"`
		Transport struct {
			Token   string `json:"token"`
			Attempt int    `json:"attempt"`
			Details struct {
				Error string `json:"error"`
			} `json:"details"`
		} `json:"transport"`
	}
	if err := json.Unmarshal([]byte(output.String()), &record); err != nil {
		t.Fatal(err)
	}
	if record.Message != "HTTP 401 "+config.RedactedValue || record.Transport.Token != config.RedactedValue ||
		record.Transport.Attempt != 2 || record.Transport.Details.Error != "initialize refused "+config.RedactedValue {
		t.Fatalf("redaction changed structured diagnostic fields: %s", output.String())
	}
}
