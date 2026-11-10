//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/hijera/foxxycode-agent/internal/version"
)

// foxxycodeInfoGet says which build serves this API and on which machine. The web
// UI shows the version in the start screen's footer, for the server the page
// is talking to - the local one, a remote or a node through a relay - and
// names the machine the page runs on by its host name on the swarm map, where
// the connection to a relay starts.
func (s *Server) foxxycodeInfoGet(w http.ResponseWriter, _ *http.Request) {
	host, _ := os.Hostname()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"object":   "foxxycode.info",
		"version":  version.Get(),
		"hostname": host,
	})
}
