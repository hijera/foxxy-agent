package mcp

import (
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// httptest's TLS servers present a certificate no system root signed, which is
// exactly the case insecure_skip_verify exists for: a self-signed or otherwise
// unverifiable MCP endpoint the operator has decided to trust anyway.

func TestStreamableHTTPRejectsSelfSignedCertificateByDefault(t *testing.T) {
	ts := httptest.NewTLSServer(&fakeStreamableHandler{})
	defer ts.Close()

	// Both the streamable attempt and its SSE fallback must fail on the cert.
	_, err := NewHTTPClient(testCtx(t), "remote", ts.URL, nil, false, slog.Default())
	if err == nil {
		t.Fatal("a self-signed certificate must fail the connect by default")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("err = %v, want a certificate error", err)
	}
}

func TestStreamableHTTPConnectsWithInsecureSkipVerify(t *testing.T) {
	ts := httptest.NewTLSServer(&fakeStreamableHandler{})
	defer ts.Close()

	client, err := NewHTTPClient(testCtx(t), "remote", ts.URL, nil, true, slog.Default())
	if err != nil {
		t.Fatalf("NewHTTPClient with insecure skip verify: %v", err)
	}
	defer func() { _ = client.Close() }()

	if tools := client.Tools(); len(tools) != 1 || tools[0].Name != "remote_echo" {
		t.Fatalf("tools = %+v, want remote_echo", client.Tools())
	}
	got, err := client.CallTool(testCtx(t), "remote_echo", `{"text":"hi"}`)
	if err != nil || got != "remote:hi" {
		t.Fatalf("CallTool = %q, %v; want remote:hi", got, err)
	}
}

func TestLegacySSERejectsSelfSignedCertificateByDefault(t *testing.T) {
	ts := httptest.NewTLSServer(fakeLegacySSEMux(t))
	defer ts.Close()

	_, err := NewSSEClient(testCtx(t), "legacy", ts.URL+"/sse", nil, false, slog.Default())
	if err == nil {
		t.Fatal("a self-signed certificate must fail the sse connect by default")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("err = %v, want a certificate error", err)
	}
}

func TestLegacySSEConnectsWithInsecureSkipVerify(t *testing.T) {
	// The GET stream and the POST endpoint are separate requests: this passes
	// only when both go through the transport's configured client.
	ts := httptest.NewTLSServer(fakeLegacySSEMux(t))
	defer ts.Close()

	client, err := NewSSEClient(testCtx(t), "legacy", ts.URL+"/sse", nil, true, slog.Default())
	if err != nil {
		t.Fatalf("NewSSEClient with insecure skip verify: %v", err)
	}
	defer func() { _ = client.Close() }()

	got, err := client.CallTool(testCtx(t), "sse_echo", `{"text":"x"}`)
	if err != nil || got != "sse:x" {
		t.Fatalf("CallTool = %q, %v; want sse:x", got, err)
	}
}

func TestConnectPropagatesInsecureSkipVerify(t *testing.T) {
	ts := httptest.NewTLSServer(&fakeStreamableHandler{})
	defer ts.Close()

	srv := config.MCPServerConfig{Name: "remote", Type: "http", URL: ts.URL}
	if _, err := Connect(testCtx(t), srv, t.TempDir(), slog.Default()); err == nil {
		t.Fatal("Connect must fail on a self-signed certificate without the flag")
	}

	srv.InsecureSkipVerify = true
	client, err := Connect(testCtx(t), srv, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatalf("Connect with insecure_skip_verify: %v", err)
	}
	defer func() { _ = client.Close() }()
	if len(client.Tools()) != 1 {
		t.Fatalf("tools = %+v, want one", client.Tools())
	}
}

func TestProbePropagatesInsecureSkipVerify(t *testing.T) {
	ts := httptest.NewTLSServer(&fakeStreamableHandler{})
	defer ts.Close()

	srv := config.MCPServerConfig{Name: "remote", Type: "http", URL: ts.URL, InsecureSkipVerify: true}
	tools, err := Probe(testCtx(t), srv, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatalf("Probe with insecure_skip_verify: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "remote_echo" {
		t.Fatalf("tools = %+v", tools)
	}
}

// TestFingerprintOfDeclarationsWithoutTheFlagIsUnchanged pins the digest of a
// declaration that does not set insecure_skip_verify to the value it had before
// the field existed. Approvals in <home>/mcp-trust.json are keyed by this
// digest, so a change here would silently revoke every stored approval.
func TestFingerprintOfDeclarationsWithoutTheFlagIsUnchanged(t *testing.T) {
	const want = "sha256:0ff705e99b87d81477853f341d6cb086d2dd17589d944bc4f21c147505f3719a"
	if got := Fingerprint(projectServer("demo", "run-me")); got != want {
		t.Fatalf("fingerprint = %s, want %s (stored approvals would be revoked)", got, want)
	}
}

// Turning verification off changes what the connection is exposed to, so it is
// part of the approved declaration rather than an operational switch.
func TestFingerprintChangesWithInsecureSkipVerify(t *testing.T) {
	base := config.MCPServerConfig{Name: "remote", Type: "http", URL: "https://mcp.example.com/mcp"}
	insecure := base
	insecure.InsecureSkipVerify = true
	if Fingerprint(insecure) == Fingerprint(base) {
		t.Fatal("disabling certificate verification must re-require approval")
	}
}

func TestTrustRecordRecordsInsecureSkipVerify(t *testing.T) {
	srv := config.MCPServerConfig{Name: "remote", Type: "http", URL: "https://mcp.example.com/mcp", InsecureSkipVerify: true}
	rec := NewTrustRecord(srv, "/ws/.foxxycode/mcp.json", time.Now())
	if !rec.InsecureSkipVerify {
		t.Fatal("the receipt must record that certificate verification is disabled")
	}
}
