//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/ideterm"
)

// terminalStateEntry is the JSON shape of a single terminal in the
// terminal-state request/response bodies.
type terminalStateEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Shell       string `json:"shell,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	LastCommand string `json:"lastCommand,omitempty"`
	Output      string `json:"output,omitempty"`
	Active      bool   `json:"active,omitempty"`
}

// foxxycodeIdeTerminalStatePost ingests the terminal state pushed by IDE
// extensions (POST /foxxycode/ide/terminal-state) and stores the latest
// snapshot so it can be injected into subsequent agent turns. The IDE reports
// every open terminal plus a bounded tail of its recent output; there is one
// foxxycode process per workspace, so the snapshot is process-global.
func (s *Server) foxxycodeIdeTerminalStatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Terminals []terminalStateEntry `json:"terminals"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	terminals := make([]ideterm.Terminal, 0, len(body.Terminals))
	for _, t := range body.Terminals {
		terminals = append(terminals, ideterm.Terminal{
			ID:          t.ID,
			Name:        t.Name,
			Shell:       t.Shell,
			Cwd:         t.Cwd,
			LastCommand: t.LastCommand,
			Output:      t.Output,
			Active:      t.Active,
		})
	}
	ideterm.Set(terminals)
	w.WriteHeader(http.StatusNoContent)
}

// foxxycodeIdeTerminalStateGet returns the currently tracked terminals (id,
// name and active flag only — no output) so the SPA can populate the @terminal
// mention menu.
func (s *Server) foxxycodeIdeTerminalStateGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	snap := ideterm.Get()
	out := struct {
		Terminals []terminalStateEntry `json:"terminals"`
	}{Terminals: make([]terminalStateEntry, 0, len(snap.Terminals))}
	for _, t := range snap.Terminals {
		out.Terminals = append(out.Terminals, terminalStateEntry{
			ID:     t.ID,
			Name:   t.Name,
			Active: t.Active,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
