// Package httpserver implements an OpenAI-compatible HTTP API for FoxxyCode.
//
// The API itself is behind the http build tag. This file is not: it carries the
// options a caller fills in and the flag that says whether this binary can serve
// at all, so `foxxycode serve` can read a configuration that asks for the API and
// answer honestly in a build that has none.
package httpserver

import (
	"log/slog"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/webauth"
)

// The environment variables that carry this surface's credentials, outside the
// build tag so `foxxycode serve` reads the same names in a binary that cannot
// serve the API and quotes them in its own help. They are defined in
// internal/webauth: the dry run reads them too, and this package imports it.
const (
	// TokenEnvVar is the bearer token for API clients.
	TokenEnvVar = webauth.TokenEnvVar
	// LoginUserEnvVar and LoginPasswordEnvVar are the web sign-in account.
	LoginUserEnvVar     = webauth.LoginUserEnvVar
	LoginPasswordEnvVar = webauth.LoginPasswordEnvVar
)

// Options are what one HTTP server instance is built from.
type Options struct {
	// Cfg is the configuration the server starts with. Later edits reach it
	// through ReplaceConfig, not through this pointer.
	Cfg *config.Config
	// Mgr owns the sessions this server serves.
	Mgr *session.Manager
	// Log is the process logger.
	Log *slog.Logger
	// DefaultCWD is the workspace a session gets when the client names none.
	DefaultCWD string
	// Home is the agent state directory, used by the swarm join credentials.
	Home string
	// ListenAddr is the already-resolved host:port to bind.
	ListenAddr string
	// ExtraAuthTokens are bearer tokens supplied out of band (--auth-token,
	// FOXXYCODE_HTTP_TOKEN) so a credential need not be written into config.yaml.
	ExtraAuthTokens []string
	// ExtraLogin is a web sign-in account supplied out of band (FOXXYCODE_HTTP_USER,
	// FOXXYCODE_HTTP_PASSWORD), for the same reason: it enables the form on its own
	// and never reaches the file, so a save from the settings screen cannot
	// write the operator's password into config.yaml.
	ExtraLogin LoginCredentials
	// OnServer, when set, is handed the live server as it comes up and nil as
	// it goes down. It is how the process installs the turn mirror, and how it
	// drops the mirror again when this subsystem restarts.
	OnServer func(*Server)
}

// LoginCredentials is one web-UI account in plaintext, as the environment
// carries it. The server hashes the password as it starts and keeps only that.
type LoginCredentials struct {
	User     string
	Password string
}

// IsSet reports whether both halves of an account are present. One without the
// other is not a credential, and is treated as none at all.
func (c LoginCredentials) IsSet() bool { return c.User != "" && c.Password != "" }
