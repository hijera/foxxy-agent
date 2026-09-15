//go:build http

package httpserver

// The two loopback rules this fork adds around the gate, which upstream has no
// counterpart for. `foxxycode http` is what the IntelliJ and VS Code plugins and
// `foxxycode desktop` start on 127.0.0.1 and call with no credential, so there a
// direct loopback client passes the web sign-in form; and on any server the IDE
// routes are open without a credential to that same client and to nobody else.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// startHTTPWith runs StartHTTP - what `foxxycode http`, the plugins and the
// desktop app go through - over a config body, without listening.
func startHTTPWith(t *testing.T, body string) (*StartedHTTP, error) {
	t.Helper()
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config.yaml")
	yml := "providers:\n  - name: openai\n    type: openai\n    api_key: \"k\"\nmodels:\n  - model: \"openai/gpt-4o\"\n    max_tokens: 4096\nagent:\n  model: \"openai/gpt-4o\"\n" + body
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := StartHTTP(CommandDeps{
		NewServerRef: func(**acp.Server, *config.Config, func() *config.Config) acp.UpdateSender { return noopSender{} },
		EnsureHome:   func(string) error { return nil },
		OpenStore: func(string, *config.Config) (*session.FileStore, error) {
			return &session.FileStore{Root: filepath.Join(home, "sessions")}, nil
		},
	}, StartParams{CLI: config.CLIPaths{Home: home, CWD: home, Config: cfgPath}, ListenAddr: "127.0.0.1:0"})
	if err == nil {
		t.Cleanup(func() { _ = st.Shutdown(context.Background()) })
	}
	return st, err
}

func TestStartHTTPTrustsADirectLoopbackClientAtTheSignInForm(t *testing.T) {
	t.Setenv(LoginUserEnvVar, "")
	t.Setenv(LoginPasswordEnvVar, "")
	t.Setenv(TokenEnvVar, "")
	hash := strings.ReplaceAll(loginTestHash(t, loginTestPassword), "$", "$$")
	st, err := startHTTPWith(t, "httpserver:\n  login:\n    user: "+loginTestUser+"\n    password_hash: \""+hash+"\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Server.loginPolicyNow().enabled {
		t.Fatal("the account in config.yaml did not turn the form on")
	}
	if got := serveStatus(st.Server, loopbackRequest(http.MethodGet, "/foxxycode/sessions", "")); got != http.StatusOK {
		t.Fatalf("foxxycode http refused the plugin on this machine: %d", got)
	}
	if got := serveStatus(st.Server, httptest.NewRequest(http.MethodGet, "/foxxycode/sessions", nil)); got != http.StatusUnauthorized {
		t.Fatalf("foxxycode http let a client on the network past the form: %d", got)
	}
}

func TestStartHTTPReadsTheSignInAccountFromTheEnvironment(t *testing.T) {
	t.Setenv(LoginUserEnvVar, "envuser")
	t.Setenv(LoginPasswordEnvVar, "env-pass")
	t.Setenv(TokenEnvVar, "")
	st, err := startHTTPWith(t, "")
	if err != nil {
		t.Fatal(err)
	}
	pol := st.Server.loginPolicyNow()
	if !pol.enabled || pol.account.user != "envuser" || pol.account.source != loginSourceEnv {
		t.Fatalf("the environment account was not used: %+v", pol)
	}
}

func TestStartHTTPRefusesASignInFormWithNoAccount(t *testing.T) {
	t.Setenv(LoginUserEnvVar, "")
	t.Setenv(LoginPasswordEnvVar, "")
	_, err := startHTTPWith(t, "httpserver:\n  login:\n    enabled: true\n")
	if err == nil || !strings.Contains(err.Error(), "no account is configured") {
		t.Fatalf("want the no-account refusal, got %v", err)
	}
}

// loopbackRequest is a request the way an editor plugin on this machine sends it.
func loopbackRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:52100"
	r.Host = "127.0.0.1:12345"
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

func serveStatus(srv *Server, r *http.Request) int {
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	return w.Code
}

func authMe(t *testing.T, srv *Server, r *http.Request) map[string]interface{} {
	t.Helper()
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /foxxycode/auth/me: %d %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestIsDirectLoopbackClient(t *testing.T) {
	cases := []struct {
		name    string
		remote  string
		host    string
		headers map[string]string
		want    bool
	}{
		{"ipv4 peer, ipv4 host with port", "127.0.0.1:50000", "127.0.0.1:12345", nil, true},
		{"ipv6 peer and host", "[::1]:50000", "[::1]:12345", nil, true},
		{"localhost host without a port", "127.0.0.1:50000", "localhost", nil, true},
		{"Host names are compared without case", "127.0.0.1:50000", "LocalHost:12345", nil, true},
		{"a peer on the network", "192.168.1.20:50000", "127.0.0.1:12345", nil, false},
		{"a name that resolves to loopback (DNS rebinding)", "127.0.0.1:50000", "attacker.example:12345", nil, false},
		{"a lookalike name", "127.0.0.1:50000", "127.0.0.1.nip.io:12345", nil, false},
		{"a proxy on this machine (X-Forwarded-For)", "127.0.0.1:50000", "127.0.0.1:12345", map[string]string{"X-Forwarded-For": "203.0.113.9"}, false},
		{"a proxy on this machine (Forwarded)", "127.0.0.1:50000", "127.0.0.1:12345", map[string]string{"Forwarded": "for=203.0.113.9"}, false},
		{"a proxy on this machine (X-Real-IP)", "127.0.0.1:50000", "127.0.0.1:12345", map[string]string{"X-Real-IP": "203.0.113.9"}, false},
		{"a proxy on this machine (X-Forwarded-Host)", "127.0.0.1:50000", "127.0.0.1:12345", map[string]string{"X-Forwarded-Host": "box.example"}, false},
		{"a peer that is not an address", "pipe", "127.0.0.1:12345", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/foxxycode/sessions", nil)
			r.RemoteAddr = tc.remote
			r.Host = tc.host
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := isDirectLoopbackClient(r); got != tc.want {
				t.Fatalf("isDirectLoopbackClient(peer %s, host %s, %v) = %v, want %v", tc.remote, tc.host, tc.headers, got, tc.want)
			}
		})
	}
}

func TestFoxxyCodeHTTPTreatsADirectLoopbackClientAsSignedIn(t *testing.T) {
	srv := newLoginServer(t, configuredLogin(t))
	srv.SetTrustLoopbackClients(true)

	if got := serveStatus(srv, loopbackRequest(http.MethodGet, "/foxxycode/sessions", "")); got != http.StatusOK {
		t.Fatalf("a plugin on this machine was refused: %d", got)
	}
	me := authMe(t, srv, loopbackRequest(http.MethodGet, "/foxxycode/auth/me", ""))
	if me["login_required"] != false || me["authenticated"] != true {
		t.Fatalf("the panel would draw a sign-in form: %v", me)
	}
	if _, ok := me["user"]; ok {
		t.Fatalf("no account signed in, so there is no user to sign out: %v", me)
	}

	// Everyone else still meets the form.
	remote := httptest.NewRequest(http.MethodGet, "/foxxycode/sessions", nil)
	if got := serveStatus(srv, remote); got != http.StatusUnauthorized {
		t.Fatalf("a client on the network passed the form: %d", got)
	}
	remoteMe := authMe(t, srv, httptest.NewRequest(http.MethodGet, "/foxxycode/auth/me", nil))
	if remoteMe["login_required"] != true || remoteMe["authenticated"] != false {
		t.Fatalf("a client on the network was told there is no form: %v", remoteMe)
	}
	proxied := loopbackRequest(http.MethodGet, "/foxxycode/sessions", "")
	proxied.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := serveStatus(srv, proxied); got != http.StatusUnauthorized {
		t.Fatalf("a request relayed by a proxy on this machine passed the form: %d", got)
	}
	rebound := loopbackRequest(http.MethodGet, "/foxxycode/sessions", "")
	rebound.Host = "attacker.example:12345"
	if got := serveStatus(srv, rebound); got != http.StatusUnauthorized {
		t.Fatalf("a page addressing its own name at 127.0.0.1 passed the form: %d", got)
	}
}

func TestServeKeepsTheSignInFormForLoopbackClients(t *testing.T) {
	srv := newLoginServer(t, configuredLogin(t))

	if got := serveStatus(srv, loopbackRequest(http.MethodGet, "/foxxycode/sessions", "")); got != http.StatusUnauthorized {
		t.Fatalf("serve let a loopback client past the form: %d", got)
	}
	me := authMe(t, srv, loopbackRequest(http.MethodGet, "/foxxycode/auth/me", ""))
	if me["login_required"] != true || me["authenticated"] != false {
		t.Fatalf("serve told a loopback client there is no form: %v", me)
	}
}

func TestLoopbackTrustDoesNotReplaceABearerToken(t *testing.T) {
	srv := newLoginServer(t, configuredLogin(t))
	srv.SetExtraAuthTokens([]string{"tok-123"})
	srv.SetTrustLoopbackClients(true)

	if got := serveStatus(srv, loopbackRequest(http.MethodGet, "/foxxycode/sessions", "")); got != http.StatusUnauthorized {
		t.Fatalf("with a token configured a loopback client must still present it: %d", got)
	}
	withToken := loopbackRequest(http.MethodGet, "/foxxycode/sessions", "")
	withToken.Header.Set("Authorization", "Bearer tok-123")
	if got := serveStatus(srv, withToken); got != http.StatusOK {
		t.Fatalf("the token was refused: %d", got)
	}
}

func TestLoopbackTrustSurvivesAConfigReload(t *testing.T) {
	srv := newLoginServer(t, configuredLogin(t))
	srv.SetTrustLoopbackClients(true)

	next := *srv.activeCfg()
	next.Agent.MaxTurns = 7
	srv.ReplaceConfig(&next)

	if got := serveStatus(srv, loopbackRequest(http.MethodGet, "/foxxycode/sessions", "")); got != http.StatusOK {
		t.Fatalf("a settings save closed the panel's gate: %d", got)
	}
}

func TestIDERoutesAreOpenToADirectLoopbackClientOnly(t *testing.T) {
	tokenSrv, _ := authTestServer(t, cfgWithAuth("s3cret"))
	gates := map[string]*Server{
		"bearer token": tokenSrv,
		"sign-in form": newLoginServer(t, configuredLogin(t)),
	}
	for name, srv := range gates {
		t.Run(name, func(t *testing.T) {
			if got := serveStatus(srv, loopbackRequest(http.MethodPost, "/foxxycode/ide/editor-state", `{}`)); got == http.StatusUnauthorized {
				t.Fatal("the editor plugin on this machine was refused an IDE route")
			}
			remote := httptest.NewRequest(http.MethodPost, "/foxxycode/ide/editor-state", strings.NewReader(`{}`))
			if got := serveStatus(srv, remote); got != http.StatusUnauthorized {
				t.Fatalf("a client on the network reached an IDE route without a credential: %d", got)
			}
			events := httptest.NewRequest(http.MethodGet, "/foxxycode/ide/events", nil)
			if got := serveStatus(srv, events); got != http.StatusUnauthorized {
				t.Fatalf("a client on the network reached the IDE event stream without a credential: %d", got)
			}
			proxied := loopbackRequest(http.MethodPost, "/foxxycode/ide/editor-state", `{}`)
			proxied.Header.Set("X-Forwarded-For", "203.0.113.9")
			if got := serveStatus(srv, proxied); got != http.StatusUnauthorized {
				t.Fatalf("a request relayed by a proxy reached an IDE route: %d", got)
			}
		})
	}

	// With no credential configured at all the server is as open as it was.
	openSrv, _ := authTestServer(t, cfgWithAuth(""))
	remote := httptest.NewRequest(http.MethodPost, "/foxxycode/ide/editor-state", strings.NewReader(`{}`))
	if got := serveStatus(openSrv, remote); got == http.StatusUnauthorized {
		t.Fatal("an open server refused an IDE route")
	}
}
