package dryrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/remote"
	"github.com/hijera/foxxycode-agent/internal/webauth"
)

const modeline = "# yaml-language-server: $schema=https://foxxycode.dev/config.schema.json\n"

// prepare writes body as <home>/config.yaml and runs the static stage.
func prepare(t *testing.T, body string) (*Prepared, string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(path, []byte(modeline+body), 0o644); err != nil {
		t.Fatal(err)
	}
	prep, rep, err := Prepare(config.CLIPaths{Home: home, CWD: home, Config: path})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prep == nil {
		var buf bytes.Buffer
		rep.Write(&buf)
		t.Fatalf("the fixture does not pass the static check:\n%s", buf.String())
	}
	return prep, home
}

// run prepares body and runs the probes, letting mut shape the request.
func run(t *testing.T, body string, mut func(*Request)) *Report {
	t.Helper()
	prep, _ := prepare(t, body)
	req := Request{Cfg: prep.Cfg, Paths: prep.Paths, Locator: prep.Locator}
	if mut != nil {
		mut(&req)
	}
	return Run(context.Background(), req)
}

func find(t *testing.T, rep *Report, path string) Check {
	t.Helper()
	for _, c := range rep.Checks {
		if c.Path == path {
			return c
		}
	}
	t.Fatalf("no check for %s in %+v", path, rep.Checks)
	return Check{}
}

func modelServer(t *testing.T, ids ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models") {
			http.NotFound(w, r)
			return
		}
		var rows []string
		for _, id := range ids {
			rows = append(rows, fmt.Sprintf(`{"id":%q}`, id))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(rows, ","))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProviderListingIsOKAndModelsAreChecked(t *testing.T) {
	srv := modelServer(t, "qwen")
	rep := run(t, fmt.Sprintf(`providers:
  - name: local
    type: openai
    api_base: %s/v1
models:
  - model: local/qwen
  - model: local/other
agent:
  model: local/qwen
`, srv.URL), nil)
	p := find(t, rep, "providers[local]")
	if p.Status != StatusOK || !strings.Contains(p.Message, "1 model") {
		t.Errorf("provider check %+v", p)
	}
	if m := find(t, rep, "models[local/qwen]"); m.Status != StatusOK {
		t.Errorf("listed model %+v", m)
	}
	other := find(t, rep, "models[local/other]")
	if other.Status != StatusWarning || !strings.Contains(other.Message, "not in the model list") || other.Line == 0 {
		t.Errorf("unlisted model %+v", other)
	}
	if rep.Errors() != 0 {
		t.Errorf("errors %d in %+v", rep.Errors(), rep.Checks)
	}
}

func TestProviderRejectedCredentialIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	rep := run(t, fmt.Sprintf("providers:\n  - name: hosted\n    type: openai\n    api_base: %s/v1\n    api_key: nope\nmodels:\n  - model: hosted/x\nagent:\n  model: hosted/x\n", srv.URL), nil)
	p := find(t, rep, "providers[hosted]")
	if p.Status != StatusError || !strings.Contains(p.Message, "HTTP 401") || !strings.Contains(p.Fix, "api_key") {
		t.Errorf("provider check %+v", p)
	}
	if p.Line != 3 {
		t.Errorf("line %d, want the provider entry (3)", p.Line)
	}
	if m := find(t, rep, "models[hosted/x]"); m.Status != StatusSkipped {
		t.Errorf("a model of a failed provider is skipped, got %+v", m)
	}
}

func TestProviderNotFoundHintsAtTheAPIPrefix(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	rep := run(t, fmt.Sprintf("providers:\n  - name: local\n    type: openai\n    api_base: %s\n", srv.URL), nil)
	p := find(t, rep, "providers[local]")
	if p.Status != StatusError || !strings.Contains(p.Message, "HTTP 404") || !strings.Contains(p.Fix, "/v1") {
		t.Errorf("provider check %+v", p)
	}
}

func TestProviderUnreachableIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	rep := run(t, fmt.Sprintf("providers:\n  - name: local\n    type: openai\n    api_base: %s/v1\n", url), nil)
	p := find(t, rep, "providers[local]")
	if p.Status != StatusError || !strings.Contains(p.Message, "cannot reach") {
		t.Errorf("provider check %+v", p)
	}
}

func TestOfficialEndpointWithoutCredentialNeedsNoNetwork(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	rep := run(t, "providers:\n  - name: openai\n    type: openai\nmodels:\n  - model: openai/gpt\nagent:\n  model: openai/gpt\n", nil)
	p := find(t, rep, "providers[openai]")
	if p.Status != StatusError || !strings.Contains(p.Message, "no credential") || !strings.Contains(p.Fix, "OPENAI_API_KEY") {
		t.Errorf("provider check %+v", p)
	}
}

func TestListenerFreeAndInUse(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	freeAddr := free.Addr().String()
	_ = free.Close()
	port := held.Addr().(*net.TCPAddr).Port
	rep := run(t, fmt.Sprintf("httpserver:\n  host: 127.0.0.1\n  port: %d\n", port), func(r *Request) {
		r.Listeners = []Listener{{Path: "httpserver", Addr: held.Addr().String()}, {Path: "swarm", Addr: freeAddr}}
	})
	h := find(t, rep, "httpserver")
	if h.Status != StatusError || !strings.Contains(h.Message, "in use") || h.Line != 4 {
		t.Errorf("held listener %+v", h)
	}
	if s := find(t, rep, "swarm"); s.Status != StatusOK || !strings.Contains(s.Message, "free") {
		t.Errorf("free listener %+v", s)
	}
}

func TestTelegramTokenProbe(t *testing.T) {
	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getMe") || !strings.Contains(r.URL.Path, "/bot123:abc/") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{"username":"dry_bot"}}`)
		} else {
			_, _ = fmt.Fprint(w, `{"ok":false,"description":"Unauthorized"}`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv(config.TelegramAPIBaseEnv, srv.URL)
	body := "gateways:\n  telegram:\n    enabled: true\n    token: \"123:abc\"\n"

	status = http.StatusOK
	if c := find(t, run(t, body, nil), "gateways.telegram"); c.Status != StatusOK || !strings.Contains(c.Message, "@dry_bot") {
		t.Errorf("accepted token %+v", c)
	}
	status = http.StatusUnauthorized
	if c := find(t, run(t, body, nil), "gateways.telegram"); c.Status != StatusError || !strings.Contains(c.Message, "rejected") || c.Line != 5 {
		t.Errorf("rejected token %+v", c)
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	if c := find(t, run(t, "gateways:\n  telegram:\n    enabled: true\n", nil), "gateways.telegram"); c.Status != StatusError || !strings.Contains(c.Message, "no token") {
		t.Errorf("missing token %+v", c)
	}
}

func TestMCPCommandLookup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookup of a shell script is a POSIX fixture")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "foxxycode-dry-run-tool"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	rep := run(t, "mcp_servers:\n  - name: found\n    command: foxxycode-dry-run-tool\n  - name: missing\n    command: definitely-not-installed-foxxycode-mcp\n  - name: off\n    command: definitely-not-installed-foxxycode-mcp\n    disabled: true\n", nil)
	if c := find(t, rep, "mcp_servers[found]"); c.Status != StatusOK || !strings.Contains(c.Message, "resolves to") {
		t.Errorf("found %+v", c)
	}
	m := find(t, rep, "mcp_servers[missing]")
	if m.Status != StatusError || !strings.Contains(m.Message, "not found") || m.Line != 6 {
		t.Errorf("missing %+v", m)
	}
	if c := find(t, rep, "mcp_servers[off]"); c.Status != StatusSkipped {
		t.Errorf("disabled %+v", c)
	}
}

func TestMCPRemoteReachability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != "secret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	t.Cleanup(srv.Close)
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()
	rep := run(t, fmt.Sprintf("mcp_servers:\n  - name: up\n    url: %s/mcp\n    headers:\n      - name: X-Token\n        value: secret\n  - name: down\n    url: %s/mcp\n", srv.URL, downURL), nil)
	if c := find(t, rep, "mcp_servers[up]"); c.Status != StatusOK || !strings.Contains(c.Message, "HTTP 405") {
		t.Errorf("up %+v", c)
	}
	if c := find(t, rep, "mcp_servers[down]"); c.Status != StatusError || !strings.Contains(c.Message, "cannot reach") {
		t.Errorf("down %+v", c)
	}
}

func TestRemoteTargetProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)
	ok := run(t, "agent:\n  max_turns: 3\n", func(r *Request) { r.Remote = &remote.Options{BaseURL: srv.URL, Token: "good"} })
	if c := find(t, ok, "--remote"); c.Status != StatusOK || !strings.Contains(c.Message, "accepts the token") {
		t.Errorf("good token %+v", c)
	}
	bad := run(t, "agent:\n  max_turns: 3\n", func(r *Request) { r.Remote = &remote.Options{BaseURL: srv.URL, Token: "bad"} })
	if c := find(t, bad, "--remote"); c.Status != StatusError || !strings.Contains(c.Message, "HTTP 401") || !strings.Contains(c.Fix, "--remote-token") {
		t.Errorf("bad token %+v", c)
	}
}

func TestConfiguredRemotesDownAreWarnings(t *testing.T) {
	down := httptest.NewServer(http.NotFoundHandler())
	url := down.URL
	down.Close()
	rep := run(t, fmt.Sprintf("httpserver:\n  remotes:\n    - name: nas\n      url: %s\n", url), nil)
	c := find(t, rep, "httpserver.remotes[nas]")
	if c.Status != StatusWarning || !strings.Contains(c.Message, "cannot reach") || c.Line != 5 {
		t.Errorf("remote %+v (line 5 is the url)", c)
	}
}

// fakeRelay answers the way a swarm relay does: no /v1 at all, a public
// /swarm/info, and the node list behind its client token.
func fakeRelay(t *testing.T, clientToken string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/swarm/info":
			_, _ = fmt.Fprint(w, `{"swarm":true,"name":"office","node_count":2}`)
		case "/swarm/nodes":
			if r.Header.Get("Authorization") != "Bearer "+clientToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = fmt.Fprint(w, `{"nodes":[{"name":"worker-a","kind":"agent","online":true},{"name":"worker-b","kind":"agent","online":false},{"name":"inner","kind":"relay","online":true}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A relay serves no /v1, so --remote at a relay's root cannot drive anything:
// the dry run says it is a relay and names the node mounts to use instead
// (issue #401), rather than "answered HTTP 404".
func TestRemoteTargetThatIsARelayNamesItsNodes(t *testing.T) {
	relay := fakeRelay(t, "client")
	rep := run(t, "agent:\n  max_turns: 3\n", func(r *Request) { r.Remote = &remote.Options{BaseURL: relay.URL, Token: "client"} })
	c := find(t, rep, "--remote")
	if c.Status != StatusError || !strings.Contains(c.Message, "swarm relay") {
		t.Fatalf("relay target %+v", c)
	}
	if !strings.Contains(c.Fix, relay.URL+"/swarm/nodes/worker-a") || !strings.Contains(c.Fix, "worker-b") || strings.Contains(c.Fix, "inner") {
		t.Errorf("the fix should name the agents' mounts: %+v", c)
	}
	refused := run(t, "agent:\n  max_turns: 3\n", func(r *Request) { r.Remote = &remote.Options{BaseURL: relay.URL, Token: "wrong"} })
	if c := find(t, refused, "--remote"); c.Status != StatusError || !strings.Contains(c.Message, "rejected the token") || !strings.Contains(c.Fix, "swarm.auth_token") {
		t.Errorf("relay refusing the token %+v", c)
	}
}

// A configured remote is probed with the token its entry carries, and a relay
// among them is recognised as one.
func TestConfiguredRemotesUseTheirTokenAndKnowARelay(t *testing.T) {
	relay := fakeRelay(t, "client")
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer box-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[]}`)
	}))
	t.Cleanup(agent.Close)
	cfg := fmt.Sprintf("httpserver:\n  remotes:\n"+
		"    - name: office\n      url: %s\n      token: client\n"+
		"    - name: office-wrong\n      url: %s\n      token: nope\n"+
		"    - name: office-bare\n      url: %s\n"+
		"    - name: box\n      url: %s\n      token: box-token\n"+
		"    - name: box-wrong\n      url: %s\n      token: nope\n",
		relay.URL, relay.URL, relay.URL, agent.URL, agent.URL)
	rep := run(t, cfg, nil)
	if c := find(t, rep, "httpserver.remotes[office]"); c.Status != StatusOK || !strings.Contains(c.Message, "swarm relay") || !strings.Contains(c.Message, "accepts the token") {
		t.Errorf("relay with its token %+v", c)
	}
	if c := find(t, rep, "httpserver.remotes[office-wrong]"); c.Status != StatusWarning || !strings.Contains(c.Message, "rejected the token") {
		t.Errorf("relay refusing the entry's token %+v", c)
	}
	// Without a token in the entry the token is the browser's or the flag's
	// business, so the relay is simply there.
	if c := find(t, rep, "httpserver.remotes[office-bare]"); c.Status != StatusOK || !strings.Contains(c.Message, "swarm relay") {
		t.Errorf("relay without a token %+v", c)
	}
	if c := find(t, rep, "httpserver.remotes[box]"); c.Status != StatusOK || !strings.Contains(c.Message, "accepts the token") {
		t.Errorf("agent with its token %+v", c)
	}
	if c := find(t, rep, "httpserver.remotes[box-wrong]"); c.Status != StatusWarning || !strings.Contains(c.Message, "rejected the token") {
		t.Errorf("agent refusing the entry's token %+v", c)
	}
}

func TestSessionsDirAndLogFile(t *testing.T) {
	state := t.TempDir()
	// The store and the logger create missing directories themselves, so a
	// missing path is fine; a path that runs through a regular file is not.
	if err := os.WriteFile(filepath.Join(state, "afile"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := run(t, fmt.Sprintf("sessions:\n  dir: %s\nlogger:\n  outputs: [file]\n  file: %s\n",
		filepath.Join(state, "sessions"), filepath.Join(state, "afile", "foxxycode.log")), nil)
	if c := find(t, rep, "sessions.dir"); c.Status != StatusOK || !strings.Contains(c.Message, "will be created") {
		t.Errorf("sessions dir %+v", c)
	}
	if c := find(t, rep, "logger.file"); c.Status != StatusError || c.Line != 6 {
		t.Errorf("log file %+v", c)
	}
}

func TestPromptsDirAndTemplates(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	rep := run(t, "prompts:\n  dir: "+missing+"\n", nil)
	if c := find(t, rep, "prompts.dir"); c.Status != StatusError || !strings.Contains(c.Message, "does not exist") || c.Line != 3 {
		t.Errorf("missing dir %+v", c)
	}
	present := t.TempDir()
	if err := os.WriteFile(filepath.Join(present, "agent.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep = run(t, "prompts:\n  dir: "+present+"\n", nil)
	if c := find(t, rep, "prompts.dir"); c.Status != StatusOK {
		t.Errorf("present dir %+v", c)
	}
	if c := find(t, rep, "prompts.plan_prompt"); c.Status != StatusWarning || !strings.Contains(c.Message, "plan.md") {
		t.Errorf("missing template %+v", c)
	}
	for _, c := range rep.Checks {
		if c.Path == "prompts.agent_prompt" {
			t.Errorf("a template that exists is not reported: %+v", c)
		}
	}
}

func TestExplicitSkillsDirMissingIsAWarningDefaultsAreSilent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "skills-gone")
	rep := run(t, "skills:\n  dirs: ['"+missing+"']\n", nil)
	if c := find(t, rep, "skills.dirs[0]"); c.Status != StatusWarning || !strings.Contains(c.Message, "does not exist") || c.Line != 3 {
		t.Errorf("explicit dir %+v", c)
	}
	rep = run(t, "agent:\n  max_turns: 3\n", nil)
	for _, c := range rep.Checks {
		if strings.HasPrefix(c.Path, "skills.dirs") && c.Status != StatusOK {
			t.Errorf("a default dir that is absent must stay quiet: %+v", c)
		}
	}
}

func TestHooksFileParses(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(good, []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := run(t, fmt.Sprintf("hooks:\n  files: [%q, %q]\n", good, bad), nil)
	if c := find(t, rep, "hooks.files[0]"); c.Status != StatusOK {
		t.Errorf("good %+v", c)
	}
	if c := find(t, rep, "hooks.files[1]"); c.Status != StatusError || c.Line != 3 {
		t.Errorf("bad %+v", c)
	}
}

func TestSwarmTLSAndCAFiles(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.pem")
	if err := os.WriteFile(junk, []byte("not a certificate"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := run(t, fmt.Sprintf("swarm:\n  enabled: true\n  auth_token: t\n  tls:\n    cert_file: %s\n    key_file: %s\n  join:\n    - url: http://127.0.0.1:9\n      pairing_token: p\n    - url: http://127.0.0.1:9\n      pairing_token: p\n      dial:\n        ca_file: %s\n", junk, junk, junk), nil)
	if c := find(t, rep, "swarm.tls"); c.Status != StatusError || c.Line != 5 {
		t.Errorf("tls %+v", c)
	}
	if c := find(t, rep, "swarm.join[0]"); c.Status != StatusError || !strings.Contains(c.Message, "cannot reach") || c.Line != 9 {
		t.Errorf("join %+v", c)
	}
	if c := find(t, rep, "swarm.join[1].dial.ca_file"); c.Status != StatusError || !strings.Contains(c.Message, "certificate") {
		t.Errorf("ca %+v", c)
	}
	if c := find(t, rep, "swarm.join[1]"); c.Status != StatusSkipped {
		t.Errorf("a peer behind an unusable CA bundle is not probed: %+v", c)
	}
}

// TestSwarmPeersAreProbedOnlyForServe pins the one surface split: joining a
// relay is what foxxycode serve does, so a dry run of the console, acp or http
// must not report a relay those commands never dial.
func TestSwarmPeersAreProbedOnlyForServe(t *testing.T) {
	body := "swarm:\n  enabled: true\n  auth_token: t\n  join:\n    - url: http://127.0.0.1:9\n      pairing_token: p\n"
	for _, surface := range []Surface{SurfaceConsole, SurfaceACP, SurfaceHTTP} {
		rep := run(t, body, func(r *Request) { r.Surface = surface })
		for _, c := range rep.Checks {
			if strings.HasPrefix(c.Path, "swarm.join") {
				t.Errorf("%s probed a relay only foxxycode serve joins: %+v", surface, c)
			}
		}
	}
	rep := run(t, body, func(r *Request) { r.Surface = SurfaceServe })
	if c := find(t, rep, "swarm.join[0]"); c.Status != StatusError || !strings.Contains(c.Message, "cannot reach") {
		t.Errorf("serve join %+v", c)
	}
}

func TestSubsystemErrorIsReported(t *testing.T) {
	rep := run(t, "agent:\n  max_turns: 3\n", func(r *Request) {
		r.SubsystemErr = errors.New("httpserver.enable is true but this binary has no httpserver support")
	})
	if c := find(t, rep, "serve"); c.Status != StatusError || !strings.Contains(c.Message, "no httpserver support") {
		t.Errorf("subsystem %+v", c)
	}
}

func TestReportWriteFormat(t *testing.T) {
	rep := &Report{File: "/x/config.yaml", Checks: []Check{
		{Status: StatusOK, Path: "providers[a]", Message: "lists 2 models"},
		{Status: StatusError, Path: "providers[b]", Message: "credential rejected", Fix: "check api_key", Line: 7, Column: 5},
		{Status: StatusWarning, Path: "skills.dirs[0]", Message: "/y does not exist"},
		{Status: StatusSkipped, Path: "models[b/x]", Message: "provider b failed"},
	}}
	var buf bytes.Buffer
	rep.Write(&buf)
	txt := buf.String()
	for _, want := range []string{
		"ok       providers[a]: lists 2 models\n",
		"error    providers[b]: credential rejected\n         at /x/config.yaml:7:5\n         fix: check api_key\n",
		"warning  skills.dirs[0]: /y does not exist\n",
		"skipped  models[b/x]: provider b failed\n",
		"dry run: 1 error, 1 warning, 1 ok\n",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("report lacks %q:\n%s", want, txt)
		}
	}
	if rep.Errors() != 1 || rep.Warnings() != 1 {
		t.Errorf("counts %d/%d", rep.Errors(), rep.Warnings())
	}
}

func TestPrepareStopsAtStaticErrors(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(path, []byte("httpserver:\n  enbaled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prep, rep, err := Prepare(config.CLIPaths{Home: home, CWD: home, Config: path})
	if err != nil {
		t.Fatal(err)
	}
	if prep != nil || rep == nil || rep.Valid() {
		t.Fatalf("a file with static errors must not be probed: prep=%v report=%+v", prep, rep)
	}
}

// loginYAMLHash spells an argon2id hash the way config.yaml carries it: every
// "$" doubled, which is how the file writes a literal dollar sign.
func loginYAMLHash(t *testing.T, plain string) string {
	t.Helper()
	h, err := webauth.HashPasswordWith(plain, webauth.HashParams{Memory: 64, Time: 1, Threads: 1, SaltLen: 8, KeyLen: 16})
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return strings.ReplaceAll(h, "$", "$$")
}

func serveRun(t *testing.T, body string) *Report {
	t.Helper()
	return run(t, body, func(r *Request) { r.Surface = SurfaceServe })
}

func TestWebLoginIsSilentWithoutAnAccount(t *testing.T) {
	t.Setenv(webauth.LoginUserEnvVar, "")
	t.Setenv(webauth.LoginPasswordEnvVar, "")
	c := find(t, serveRun(t, "httpserver:\n  host: 127.0.0.1\n"), "httpserver.login")
	if c.Status != StatusSkipped {
		t.Fatalf("a server with no sign-in should be skipped, got %+v", c)
	}
}

func TestWebLoginWithoutATokenWarnsAboutAPIClients(t *testing.T) {
	// The one thing an operator must not discover from a broken relay: a
	// password closes the browser, and leaves everything else with no
	// credential at all.
	t.Setenv(webauth.LoginUserEnvVar, "")
	t.Setenv(webauth.LoginPasswordEnvVar, "")
	t.Setenv(webauth.TokenEnvVar, "")
	body := "httpserver:\n  login:\n    enabled: true\n    user: pasha\n    password_hash: \"" + loginYAMLHash(t, "pw") + "\"\n"
	c := find(t, serveRun(t, body), "httpserver.login")
	if c.Status != StatusWarning || !strings.Contains(c.Message, "no bearer token") {
		t.Fatalf("want a warning about API clients, got %+v", c)
	}
	if !strings.Contains(c.Fix, webauth.TokenEnvVar) {
		t.Fatalf("the fix does not name the token: %+v", c)
	}
}

func TestWebLoginWithATokenIsOK(t *testing.T) {
	t.Setenv(webauth.LoginUserEnvVar, "")
	t.Setenv(webauth.LoginPasswordEnvVar, "")
	t.Setenv(webauth.TokenEnvVar, "")
	body := "httpserver:\n  auth_token: \"a-token\"\n  login:\n    enabled: true\n    user: pasha\n    password_hash: \"" + loginYAMLHash(t, "pw") + "\"\n"
	c := find(t, serveRun(t, body), "httpserver.login")
	if c.Status != StatusOK || !strings.Contains(c.Message, "config") {
		t.Fatalf("want an ok naming the source, got %+v", c)
	}
}

func TestWebLoginFromTheEnvironmentIsSeen(t *testing.T) {
	t.Setenv(webauth.LoginUserEnvVar, "envuser")
	t.Setenv(webauth.LoginPasswordEnvVar, "env-pass")
	t.Setenv(webauth.TokenEnvVar, "a-token")
	c := find(t, serveRun(t, "httpserver:\n  host: 0.0.0.0\n"), "httpserver.login")
	if c.Status != StatusOK || !strings.Contains(c.Message, "env") {
		t.Fatalf("an account in the environment was not seen: %+v", c)
	}
}

func TestWebLoginEnabledWithNoAccountIsAnError(t *testing.T) {
	// This is the configuration the server refuses to start on, so the dry run
	// has to name it before the restart rather than after.
	t.Setenv(webauth.LoginUserEnvVar, "")
	t.Setenv(webauth.LoginPasswordEnvVar, "")
	c := find(t, serveRun(t, "httpserver:\n  login:\n    enabled: true\n"), "httpserver.login")
	if c.Status != StatusError || !strings.Contains(c.Message, "no account") {
		t.Fatalf("want an error, got %+v", c)
	}
	if !strings.Contains(c.Fix, "set-password") {
		t.Fatalf("the fix does not say how to make an account: %+v", c)
	}
}

// foxxycode http refuses a form with no account at start just as serve does, so
// its dry run names that configuration too.
func TestWebLoginIsCheckedForTheHTTPCommand(t *testing.T) {
	t.Setenv(webauth.LoginUserEnvVar, "")
	t.Setenv(webauth.LoginPasswordEnvVar, "")
	const body = "httpserver:\n  login:\n    enabled: true\n"
	rep := run(t, body, func(r *Request) { r.Surface = SurfaceHTTP })
	if c := find(t, rep, "httpserver.login"); c.Status != StatusError || !strings.Contains(c.Message, "no account") {
		t.Fatalf("foxxycode http --dry-run: want the no-account error, got %+v", c)
	}
	for _, surface := range []Surface{SurfaceConsole, SurfaceACP} {
		rep := run(t, body, func(r *Request) { r.Surface = surface })
		for _, c := range rep.Checks {
			if c.Path == "httpserver.login" || strings.HasPrefix(c.Path, "httpserver.login.") {
				t.Errorf("%s reported a sign-in it never serves: %+v", surface, c)
			}
		}
	}
}

func TestWebLoginDisabledIsReportedAsOff(t *testing.T) {
	t.Setenv(webauth.LoginUserEnvVar, "envuser")
	t.Setenv(webauth.LoginPasswordEnvVar, "env-pass")
	c := find(t, serveRun(t, "httpserver:\n  login:\n    enabled: false\n"), "httpserver.login")
	if c.Status != StatusSkipped || !strings.Contains(c.Message, "switched off") {
		t.Fatalf("enabled: false should report as off, got %+v", c)
	}
}

func TestWebLoginHalfAnEnvironmentAccountIsAWarning(t *testing.T) {
	t.Setenv(webauth.LoginUserEnvVar, "envuser")
	t.Setenv(webauth.LoginPasswordEnvVar, "")
	t.Setenv(webauth.TokenEnvVar, "a-token")
	body := "httpserver:\n  auth_token: \"a-token\"\n  login:\n    user: pasha\n    password_hash: \"" + loginYAMLHash(t, "pw") + "\"\n"
	rep := serveRun(t, body)
	var warned bool
	for _, c := range rep.Checks {
		if c.Path == "httpserver.login" && c.Status == StatusWarning && strings.Contains(c.Message, "only one of") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("half an account in the environment was not named:\n%+v", rep.Checks)
	}
}

func TestWebLoginIsNotCheckedOutsideServe(t *testing.T) {
	// The console and acp start no HTTP surface, so the question does not apply.
	t.Setenv(webauth.LoginUserEnvVar, "envuser")
	t.Setenv(webauth.LoginPasswordEnvVar, "env-pass")
	rep := run(t, "httpserver:\n  host: 0.0.0.0\n", func(r *Request) { r.Surface = SurfaceConsole })
	for _, c := range rep.Checks {
		if c.Path == "httpserver.login" {
			t.Fatalf("a console dry run reported the web sign-in: %+v", c)
		}
	}
}

func TestCodexMaxTokensIsAWarningOnItsLine(t *testing.T) {
	// The provider probes still run: keep the codex one on a closed local port.
	t.Setenv("FOXXYCODE_CODEX_BASE_URL", "http://127.0.0.1:9")
	body := "providers:\n  - name: codex\n    type: codex\n  - name: local\n    type: openai\n    api_base: http://127.0.0.1:9/v1\n" +
		"models:\n  - model: codex/gpt-5.5\n    max_tokens: 4096\n  - model: local/qwen\n    max_tokens: 4096\n" +
		"agent:\n  model: codex/gpt-5.5\n"
	rep := run(t, body, nil)
	c := find(t, rep, "models[codex/gpt-5.5].max_tokens")
	// Line 10: the fixture's line 9 below the modeline prepare writes first.
	if c.Status != StatusWarning || !strings.Contains(c.Message, "bounds nothing") || c.Line != 10 || c.Fix != "remove max_tokens from this model" {
		t.Errorf("codex max_tokens check %+v", c)
	}
	for _, c := range rep.Checks {
		if c.Path == "models[local/qwen].max_tokens" {
			t.Errorf("an openai model's max_tokens is sent and must stay quiet: %+v", c)
		}
	}
}

// TestCodexRowOffTheCLILoginIsNamed: the Codex CLI login serves one codex row.
// Another row without a login of its own is reported as not signed in, naming
// the row the CLI login serves, before any request goes out.
func TestCodexRowOffTheCLILoginIsNamed(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"at","refresh_token":"rt","account_id":"acct-cli"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := modelServer(t, "gpt-5.5")
	t.Setenv("FOXXYCODE_CODEX_BASE_URL", backend.URL)
	rep := run(t, "providers:\n  - name: codex\n    type: codex\n  - name: codex-work\n    type: codex\nmodels:\n  - model: codex/gpt-5.5\n  - model: codex-work/gpt-5.5\nagent:\n  model: codex/gpt-5.5\n", nil)
	p := find(t, rep, "providers[codex-work]")
	if p.Status != StatusError || !strings.Contains(p.Message, "not signed in") || !strings.Contains(p.Message, `the row "codex"`) ||
		!strings.Contains(p.Fix, "foxxycode providers login codex-work") {
		t.Errorf("codex-work check %+v, want not signed in, naming the row the CLI login serves", p)
	}
	if p := find(t, rep, "providers[codex]"); p.Status != StatusOK {
		t.Errorf("codex check %+v, want the CLI login to serve it", p)
	}
}
