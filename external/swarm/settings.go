//go:build swarm

package swarm

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/configapi"
	swarmdto "github.com/hijera/foxxycode-agent/internal/swarm"
)

// EnableSettings serves the relay's own settings page (issue #401): the
// routes the web UI's Settings drawer reads and saves the configuration
// through, behind the client token like everything else but discovery. The
// form shows the relay's deployment and its log (config.RelayUISchemaMap);
// the credentials in it are write-only, as the configuration document serves
// them, and a save puts the new configuration in place through install, where
// the supervisor rebuilds the relay on it.
func (s *Server) EnableSettings(live func() *config.Config, install func(*config.Config) error) {
	if live == nil || install == nil {
		return
	}
	backend := &configapi.Backend{
		Live:    live,
		Install: install,
		Schema:  config.RelayUISchemaJSON,
		// A client token given by flag or environment is nowhere in the file,
		// and the form should not say that none is set.
		Decorate: func(dto *config.ConfigJSON) {
			dto.Swarm.AuthConfigured = len(s.clientTokens()) > 0
		},
		Revisions: configapi.NewRevisions(),
		// The form is the relay's deployment and its log, and so is the
		// document: the rest of the host's configuration (a provider's key,
		// the HTTP server) is neither served nor taken here.
		Sections: []string{"swarm", "logger"},
		Log:      s.log,
	}
	backend.Register(s.mux)
}

// relaySettingsWrite reports a request that would change a relay's settings.
//
// A relay chained under another one is reached through the parent's mount
// like any node, and the mount carries /foxxycode/* because that is where a node's
// API lives. The settings of the relay behind it are not that: its tokens and
// its registration rules decide who joins it, and a client of the parent
// relay is not its operator, the same reason registration is refused through
// a mount. Reading them is ordinary; the credentials in them are write-only.
//
// rest is the mount's remainder as it arrived, still escaped. The node routes
// on the decoded path, so the route is judged decoded too: /foxxycode/%63onfig is
// /foxxycode/config to the node. mountRemainder has already refused a remainder
// that does not decode.
func relaySettingsWrite(method, rest string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	path, err := url.PathUnescape(rest)
	if err != nil {
		return true
	}
	return path == "/foxxycode/config" || strings.HasPrefix(path, "/foxxycode/config/")
}

// refuseRelaySettingsWrite answers a settings write aimed at a relay through
// this relay's mount, and reports whether it did.
func refuseRelaySettingsWrite(w http.ResponseWriter, r *http.Request, node Node, rest string) bool {
	if node.Info.Kind != swarmdto.KindRelay || !relaySettingsWrite(r.Method, rest) {
		return false
	}
	writeError(w, http.StatusForbidden, "the settings of a relay are changed on that relay, not through the relay it joined")
	return true
}
