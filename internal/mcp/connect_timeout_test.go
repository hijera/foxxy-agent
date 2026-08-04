package mcp

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// TestHelperSilentMCPServer is not a real test: with GO_WANT_SILENT_MCP_HELPER=1 it becomes an
// MCP server that reads its input and never answers - a cold `npx` server still downloading its
// package, or one that hangs on startup.
func TestHelperSilentMCPServer(t *testing.T) {
	if os.Getenv("GO_WANT_SILENT_MCP_HELPER") != "1" {
		t.Skip("helper process")
	}
	in := bufio.NewReader(os.Stdin)
	for {
		if _, err := in.ReadBytes('\n'); err != nil {
			return
		}
	}
}

func silentServerConfig(name string) config.MCPServerConfig {
	return config.MCPServerConfig{
		Name:    name,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestHelperSilentMCPServer"},
		Env:     []config.EnvVarConfig{{Name: "GO_WANT_SILENT_MCP_HELPER", Value: "1"}},
	}
}

// A server that never completes the handshake must not block its caller forever. Before the
// bound in Connect, the initialize call simply waited on the caller's context - which, on the
// session-load path, is an HTTP request that a panel is waiting on.
func TestConnectBoundsTheHandshakeOnASilentServer(t *testing.T) {
	restore := connectTimeout
	connectTimeout = 300 * time.Millisecond
	t.Cleanup(func() { connectTimeout = restore })

	started := time.Now()
	client, err := Connect(context.Background(), silentServerConfig("cold"), t.TempDir(), slog.Default())
	elapsed := time.Since(started)

	if err == nil {
		_ = client.Close()
		t.Fatal("expected a silent server to fail the handshake")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Connect took %s; the handshake is not bounded", elapsed)
	}
}

// A caller with a tighter deadline than connectTimeout still wins.
func TestConnectHonoursATighterCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	client, err := Connect(ctx, silentServerConfig("cold"), t.TempDir(), slog.Default())
	elapsed := time.Since(started)

	if err == nil {
		_ = client.Close()
		t.Fatal("expected the caller deadline to abort the handshake")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Connect took %s; the caller deadline was ignored", elapsed)
	}
}
