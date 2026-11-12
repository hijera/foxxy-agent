package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// EffectiveTransport reports the transport Connect will use for srv: the
// explicit type when set, else "http" for url-only entries, else "stdio".
func EffectiveTransport(srv config.MCPServerConfig) string {
	typ := strings.ToLower(strings.TrimSpace(srv.Type))
	if typ != "" {
		return typ
	}
	if strings.TrimSpace(srv.Command) == "" && strings.TrimSpace(srv.URL) != "" {
		return "http"
	}
	return "stdio"
}

// SupportedTransport reports whether Connect can handle the transport name.
func SupportedTransport(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "stdio", "http", "sse", "streamable-http", "streamable_http":
		return true
	default:
		return false
	}
}

// connectTimeout bounds one Connect: transport setup plus the initialize / tools-list
// handshake. A cold npx or uvx server downloads its package on first run, so this is generous
// compared with the probe timeout used by the management UI — it exists so that a server which
// never answers cannot hold a session (and with it, a whole panel) hostage forever. A caller
// with a tighter deadline still wins. Variable so tests can shorten it.
var connectTimeout = 30 * time.Second

// Connect establishes a client for one configured MCP server, dispatching on
// its transport type: "stdio" (default) runs a local command, "http" (also
// accepted: "streamable-http", "streamable_http") speaks streamable HTTP with
// a legacy-SSE fallback, "sse" forces the legacy HTTP+SSE transport. The
// command, args, env, headers and url resolve as config.ExpandMCPValue says,
// the same for every declaration: ${CWD} against cwd, ${NAME} from the
// environment.
//
// The bound applies to the handshake only. A stdio server's *process* is deliberately
// detached from ctx (see newStdioTransport), and a handshake that times out closes the client,
// which kills and reaps that process.
func Connect(ctx context.Context, srv config.MCPServerConfig, cwd string, log *slog.Logger) (client *Client, err error) {
	defer func() { err = redactError(srv, cwd, err) }()
	if log != nil {
		log = slog.New(redactingHandler{Handler: log.Handler(), server: srv, cwd: cwd})
	}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	switch EffectiveTransport(srv) {
	case "stdio":
		if srv.InsecureSkipVerify {
			log.Warn("mcp insecure_skip_verify has no effect on the stdio transport", "server", srv.Name)
		}
		if strings.TrimSpace(srv.Command) == "" {
			return nil, fmt.Errorf("mcp %s: command is required for stdio transport", srv.Name)
		}
		command, args, env := stdioSpec(srv, cwd)
		return NewStdioClient(ctx, srv.Name, command, args, env, log)
	case "http", "streamable-http", "streamable_http":
		return NewHTTPClient(ctx, srv.Name, config.ExpandMCPValue(srv.URL, cwd), expandHeaders(srv, cwd), srv.InsecureSkipVerify, log)
	case "sse":
		return NewSSEClient(ctx, srv.Name, config.ExpandMCPValue(srv.URL, cwd), expandHeaders(srv, cwd), srv.InsecureSkipVerify, log)
	default:
		return nil, fmt.Errorf("unsupported MCP transport: %s", srv.Type)
	}
}

// stdioSpec resolves the placeholders of the command, its arguments and its
// environment (config.ExpandMCPValue: ${CWD} against the session cwd, so a
// project-local server binary follows the workspace like its arguments do,
// ${NAME} from the environment, a leading ~).
func stdioSpec(srv config.MCPServerConfig, cwd string) (command string, args, env []string) {
	command = config.ExpandMCPValue(srv.Command, cwd)
	args = make([]string, len(srv.Args))
	for i, a := range srv.Args {
		args[i] = config.ExpandMCPValue(a, cwd)
	}
	env = make([]string, len(srv.Env))
	for i, e := range srv.Env {
		env[i] = e.Name + "=" + config.ExpandMCPValue(e.Value, cwd)
	}
	return command, args, env
}

func expandHeaders(srv config.MCPServerConfig, cwd string) map[string]string {
	if len(srv.Headers) == 0 {
		return nil
	}
	headers := make(map[string]string, len(srv.Headers))
	for _, h := range srv.Headers {
		headers[h.Name] = config.ExpandMCPValue(h.Value, cwd)
	}
	return headers
}
