//go:build http

package httpserver

import (
	"context"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/mcp/mcptest"
)

func TestStartHTTPStartsAndStopsGlobalMCPServers(t *testing.T) {
	defer bgtask.Default().SetDraining(false)
	body, err := yaml.Marshal(map[string]interface{}{
		"mcp_servers": []config.MCPServerConfig{mcptest.Stdio("shared", "token")},
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := startHTTPWith(t, string(body))
	if err != nil {
		t.Fatalf("StartHTTP: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		servers := st.Manager.MCPPoolServers()
		if len(servers) == 1 && servers[0].Connected && servers[0].Kept {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("global server was not started eagerly: %+v", servers)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := st.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if got := len(st.Manager.MCPPoolServers()); got != 0 {
		t.Fatalf("running servers after Shutdown = %d, want 0", got)
	}
}
