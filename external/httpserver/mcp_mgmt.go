//go:build http

package httpserver

// MCP management REST surface (/foxxycode/mcp*): merged server list (config.yaml
// + global <home>/mcp.json + project .foxxycode/mcp.json) with tool inventories
// probed over each server's transport, server/tool disable toggles persisted
// into the owning file, and CRUD for mcp.json entries in either scope.
// Mirrors the skills management surface in skills_mgmt.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/mcp"
)

// mcpProbeTimeout bounds one server probe (spawn + initialize + tools/list).
const mcpProbeTimeout = 8 * time.Second

func (s *Server) registerMCPManagementRoutes() {
	s.mux.HandleFunc("GET /foxxycode/mcp", s.foxxycodeMCPGet)
	s.mux.HandleFunc("POST /foxxycode/mcp/{name}/enable", s.foxxycodeMCPServerToggle(false))
	s.mux.HandleFunc("POST /foxxycode/mcp/{name}/disable", s.foxxycodeMCPServerToggle(true))
	s.mux.HandleFunc("POST /foxxycode/mcp/{name}/trust", s.foxxycodeMCPServerTrust)
	s.mux.HandleFunc("POST /foxxycode/mcp/{name}/untrust", s.foxxycodeMCPServerUntrust)
	s.mux.HandleFunc("POST /foxxycode/mcp/project-trust", s.foxxycodeMCPProjectTrust)
	s.mux.HandleFunc("POST /foxxycode/mcp/{name}/tools/{tool}/enable", s.foxxycodeMCPToolToggle(false))
	s.mux.HandleFunc("POST /foxxycode/mcp/{name}/tools/{tool}/disable", s.foxxycodeMCPToolToggle(true))
	s.mux.HandleFunc("PUT /foxxycode/mcp/{name}", s.foxxycodeMCPServerPut)
	s.mux.HandleFunc("DELETE /foxxycode/mcp/{name}", s.foxxycodeMCPServerDelete)
}

// mcpToolRow is one tool of a server in the list response.
type mcpToolRow struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// mcpServerRow is one merged server in the list response.
type mcpServerRow struct {
	Name      string            `json:"name"`
	Source    string            `json:"source"`    // global | local (scope)
	Origin    string            `json:"origin"`    // config | home | project (owning file)
	Readonly  bool              `json:"readonly"`  // config.yaml entries: no edit/delete here
	Transport string            `json:"transport"` // stdio | http
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	// InsecureSkipVerify: this server's TLS certificate is not verified.
	InsecureSkipVerify bool         `json:"insecure_skip_verify,omitempty"`
	SourcePath         string       `json:"source_path,omitempty"` // file that defines the entry
	Enabled            bool         `json:"enabled"`
	Status             string       `json:"status"` // connected | error | disabled | unsupported | needs_approval | denied
	Error              string       `json:"error,omitempty"`
	Trusted            bool         `json:"trusted"`               // false only for a gated project entry
	Gated              bool         `json:"gated"`                 // the workspace trust gate applies to this entry
	Fingerprint        string       `json:"fingerprint,omitempty"` // digest an approval binds to
	Tools              []mcpToolRow `json:"tools"`
	DisabledTools      []string     `json:"disabled_tools,omitempty"`
}

// mcpProbeEntry caches one server's probed tool list keyed by its config and
// workspace fingerprint, so repeated GETs do not respawn subprocesses.
type mcpProbeEntry struct {
	fingerprint string
	tools       []mcp.ToolInfo
	err         string
}

func mcpFingerprint(srv config.MCPServerConfig, cwd string) string {
	b, _ := json.Marshal(struct {
		Server config.MCPServerConfig `json:"server"`
		CWD    string                 `json:"cwd"`
	}{Server: srv, CWD: cwd})
	return string(b)
}

// probeMCPServer returns the cached tool list for srv, probing on fingerprint
// change or when refresh is forced. The probe runs through the trust gate, so
// listing servers never starts a project command the operator has not
// approved.
func (s *Server) probeMCPServer(ctx context.Context, gate *mcp.TrustGate, srv mcp.ManagedServer, cwd string, refresh bool) ([]mcp.ToolInfo, string) {
	fp := mcpFingerprint(srv.Config, cwd)
	s.mcpProbeMu.Lock()
	if s.mcpProbeCache == nil {
		s.mcpProbeCache = make(map[string]mcpProbeEntry)
	}
	entry, ok := s.mcpProbeCache[srv.Config.Name]
	s.mcpProbeMu.Unlock()
	if ok && !refresh && entry.fingerprint == fp {
		return entry.tools, entry.err
	}

	probeCtx, cancel := context.WithTimeout(ctx, mcpProbeTimeout)
	defer cancel()
	tools, err := gate.Probe(probeCtx, srv, cwd, s.log)
	entry = mcpProbeEntry{fingerprint: fp, tools: tools}
	if err != nil {
		entry.err = err.Error()
	}
	s.mcpProbeMu.Lock()
	s.mcpProbeCache[srv.Config.Name] = entry
	s.mcpProbeMu.Unlock()
	return entry.tools, entry.err
}

func (s *Server) invalidateMCPProbe(name string) {
	s.mcpProbeMu.Lock()
	delete(s.mcpProbeCache, name)
	s.mcpProbeMu.Unlock()
}

// foxxycodeMCPGet lists merged MCP servers with tools and per-tool switches.
// ?refresh=1 re-probes every enabled stdio server.
func (s *Server) foxxycodeMCPGet(w http.ResponseWriter, r *http.Request) {
	cwd := s.sessionDefaultCWD()
	servers, err := mcp.ListManagedServers(s.activeCfg(), cwd)
	if err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"
	gate := mcp.NewTrustGate(s.activeCfg())

	rows := make([]mcpServerRow, len(servers))
	var wg sync.WaitGroup
	for i, srv := range servers {
		transport := mcp.EffectiveTransport(srv.Config)
		trust := gate.Evaluate(cwd, srv)
		row := mcpServerRow{
			Name:               srv.Config.Name,
			Source:             srv.Scope,
			Origin:             srv.Origin,
			Readonly:           srv.Origin == mcp.OriginConfig,
			Transport:          transport,
			Command:            srv.Config.Command,
			Args:               srv.Config.Args,
			URL:                srv.Config.URL,
			InsecureSkipVerify: srv.Config.InsecureSkipVerify,
			SourcePath:         mcpSourcePath(s.activeCfg(), cwd, srv.Origin),
			Enabled:            !srv.Config.Disabled,
			Trusted:            trust == mcp.TrustStateAllowed,
			Gated:              srv.Origin == mcp.OriginProject,
			Fingerprint:        mcp.Fingerprint(srv.Config),
			Tools:              []mcpToolRow{},
			DisabledTools:      srv.Config.DisabledTools,
		}
		if len(srv.Config.Env) > 0 {
			row.Env = make(map[string]string, len(srv.Config.Env))
			for _, e := range srv.Config.Env {
				row.Env[e.Name] = e.Value
			}
		}
		if len(srv.Config.Headers) > 0 {
			row.Headers = make(map[string]string, len(srv.Config.Headers))
			for _, h := range srv.Config.Headers {
				row.Headers[h.Name] = h.Value
			}
		}
		if !mcp.SupportedTransport(transport) {
			row.Status = "unsupported"
			row.Error = fmt.Sprintf("unsupported MCP transport: %s", transport)
			rows[i] = row
			continue
		}
		if srv.Config.Disabled {
			row.Status = "disabled"
			rows[i] = row
			continue
		}
		// A gated project entry is reported, never probed: probing it would
		// start exactly the command the approval is about.
		if trust != mcp.TrustStateAllowed {
			row.Status = string(trust)
			row.Error = gate.Check(cwd, srv).Error()
			rows[i] = row
			continue
		}
		rows[i] = row
		wg.Add(1)
		go func(i int, managed mcp.ManagedServer) {
			defer wg.Done()
			cfgSrv := managed.Config
			tools, probeErr := s.probeMCPServer(r.Context(), gate, managed, cwd, refresh)
			disabled := make(map[string]bool, len(cfgSrv.DisabledTools))
			for _, t := range cfgSrv.DisabledTools {
				disabled[t] = true
			}
			toolRows := make([]mcpToolRow, 0, len(tools))
			for _, t := range tools {
				toolRows = append(toolRows, mcpToolRow{Name: t.Name, Description: t.Description, Enabled: !disabled[t.Name]})
			}
			sort.Slice(toolRows, func(a, b int) bool { return toolRows[a].Name < toolRows[b].Name })
			rows[i].Tools = toolRows
			if probeErr != "" {
				rows[i].Status = "error"
				rows[i].Error = probeErr
			} else {
				rows[i].Status = "connected"
			}
		}(i, srv)
	}
	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":        "foxxycode.mcp_list",
		"workspace":     cwd,
		"project_trust": gate.Policy(),
		"items":         rows,
	})
}

// mcpSourcePath names the file a merged entry comes from, so an approval
// dialog can show where the declaration was read.
func mcpSourcePath(cfg *config.Config, cwd, origin string) string {
	switch origin {
	case mcp.OriginProject:
		return config.MCPJSONPath(cwd)
	case mcp.OriginHome:
		return config.GlobalMCPJSONPath(cfg.Paths.Home)
	default:
		return cfg.Paths.ConfigPath
	}
}

// foxxycodeMCPServerTrust approves the current declaration of a project-local
// server for this workspace, so sessions may start it.
func (s *Server) foxxycodeMCPServerTrust(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cwd := s.sessionDefaultCWD()
	servers, err := mcp.ListManagedServers(s.activeCfg(), cwd)
	if err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	gate := mcp.NewTrustGate(s.activeCfg())
	for _, srv := range servers {
		if srv.Config.Name != name {
			continue
		}
		if err := gate.Approve(cwd, srv); err != nil {
			writeFoxxyCodeMCPErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.invalidateMCPProbe(name)
		slog.Info("mcp server approved for workspace",
			"name", name, "workspace", cwd, "digest", mcp.Fingerprint(srv.Config))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":          true,
			"fingerprint": mcp.Fingerprint(srv.Config),
		})
		return
	}
	writeFoxxyCodeMCPErr(w, http.StatusBadRequest, fmt.Sprintf("mcp server %q not found", name))
}

// foxxycodeMCPServerUntrust withdraws an approval; running sessions keep their
// already connected clients, new sessions do not start the server again.
func (s *Server) foxxycodeMCPServerUntrust(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cwd := s.sessionDefaultCWD()
	removed, err := mcp.NewTrustGate(s.activeCfg()).Revoke(cwd, name)
	if err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.invalidateMCPProbe(name)
	slog.Info("mcp server approval revoked", "name", name, "workspace", cwd, "removed", removed)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "removed": removed})
}

// foxxycodeMCPProjectTrust persists the mcp.project_trust policy, so the MCP
// tab owns the whole policy rather than sending operators to another settings
// tab.
func (s *Server) foxxycodeMCPProjectTrust(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Policy string `json:"policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := mcp.SetProjectTrust(s.activeCfg(), body.Policy); err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.reloadConfigFromDisk()
	slog.Info("mcp project trust policy set", "policy", body.Policy)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":            true,
		"project_trust": s.activeCfg().MCP.ResolvedProjectTrust(),
	})
}

// foxxycodeMCPServerToggle enables or disables a whole server, persisting into
// the file that defines it (config.yaml or .foxxycode/mcp.json).
func (s *Server) foxxycodeMCPServerToggle(disable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := mcp.SetServerDisabled(s.activeCfg(), s.sessionDefaultCWD(), name, disable); err != nil {
			writeFoxxyCodeMCPErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.reloadConfigFromDisk()
		slog.Info("mcp server toggled", "name", name, "disabled", disable)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}
}

// foxxycodeMCPToolToggle enables or disables a single tool of a server.
func (s *Server) foxxycodeMCPToolToggle(disable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		tool := r.PathValue("tool")
		if strings.TrimSpace(tool) == "" {
			writeFoxxyCodeMCPErr(w, http.StatusBadRequest, "missing tool name")
			return
		}
		if err := mcp.SetToolDisabled(s.activeCfg(), s.sessionDefaultCWD(), name, tool, disable); err != nil {
			writeFoxxyCodeMCPErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.reloadConfigFromDisk()
		slog.Info("mcp tool toggled", "server", name, "tool", tool, "disabled", disable)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}
}

// foxxycodeMCPServerPut creates or updates a server entry in the mcp.json file
// selected by ?scope=: "local" (default) writes <cwd>/.foxxycode/mcp.json,
// "global" writes <home>/mcp.json.
func (s *Server) foxxycodeMCPServerPut(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := mcp.ValidateServerName(name); err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusBadRequest, err.Error())
		return
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = mcp.ScopeLocal
	}
	var entry config.MCPJSONServer
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(entry.Command) == "" && strings.TrimSpace(entry.URL) == "" {
		writeFoxxyCodeMCPErr(w, http.StatusBadRequest, "either command or url is required")
		return
	}
	if err := mcp.UpsertServer(s.activeCfg(), s.sessionDefaultCWD(), name, scope, entry); err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.invalidateMCPProbe(name)
	slog.Info("mcp server saved", "name", name, "scope", scope)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// foxxycodeMCPServerDelete removes an mcp.json-defined server from its owning
// file. Config.yaml-defined servers are refused (edit Settings instead).
func (s *Server) foxxycodeMCPServerDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := mcp.DeleteServer(s.activeCfg(), s.sessionDefaultCWD(), name); err != nil {
		writeFoxxyCodeMCPErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.invalidateMCPProbe(name)
	slog.Info("mcp server deleted", "name", name)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func writeFoxxyCodeMCPErr(w http.ResponseWriter, code int, msg string) {
	body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": msg}})
	http.Error(w, string(body), code)
}
