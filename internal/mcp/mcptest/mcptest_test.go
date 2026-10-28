package mcptest_test

import (
	"context"
	"log/slog"
	"os/exec"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/mcp"
	"github.com/hijera/foxxycode-agent/internal/mcp/mcptest"
)

func TestHelperMCPTestServer(t *testing.T) { mcptest.Main() }

// TestEveryTransportAnswersItsToken connects to one server of each kind the
// way a session does and calls its tool.
func TestEveryTransportAnswersItsToken(t *testing.T) {
	httpSrv := mcptest.NewHTTPServer("HTTP-TOKEN")
	defer httpSrv.Close()
	sseSrv := mcptest.NewSSEServer("SSE-TOKEN")
	defer sseSrv.Close()
	for _, tc := range []struct {
		srv   config.MCPServerConfig
		token string
	}{
		{mcptest.Stdio("native", "STDIO-TOKEN"), "STDIO-TOKEN"},
		{config.MCPServerConfig{Name: "remote", Type: "http", URL: httpSrv.URL}, "HTTP-TOKEN"},
		{config.MCPServerConfig{Name: "legacy", Type: "sse", URL: sseSrv.URL}, "SSE-TOKEN"},
	} {
		t.Run(tc.srv.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			client, err := mcp.Connect(ctx, tc.srv, t.TempDir(), slog.Default())
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			defer func() { _ = client.Close() }()
			if tools := client.Tools(); len(tools) != 1 || tools[0].Name != mcptest.Tool {
				t.Fatalf("tools = %+v, want [%s]", tools, mcptest.Tool)
			}
			got, err := client.CallTool(ctx, mcptest.Tool, `{}`)
			if err != nil || got != tc.token {
				t.Fatalf("CallTool = %q, %v; want %q", got, err, tc.token)
			}
		})
	}
	if httpSrv.Calls() != 1 || sseSrv.Calls() != 1 {
		t.Fatalf("calls: http %d, sse %d; want one each", httpSrv.Calls(), sseSrv.Calls())
	}
}

// TestNPXPackageAnswersItsToken starts the local package through the real
// npx, which is how most published MCP servers are run.
func TestNPXPackageAnswersItsToken(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx is not on PATH")
	}
	srv, err := mcptest.NPX("packaged", "NPX-TOKEN", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := mcp.Connect(ctx, srv, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()
	got, err := client.CallTool(ctx, mcptest.Tool, `{}`)
	if err != nil || got != "NPX-TOKEN" {
		t.Fatalf("CallTool = %q, %v; want NPX-TOKEN", got, err)
	}
}
