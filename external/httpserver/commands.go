//go:build http

package httpserver

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/external/scheduler"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/logger"
	"github.com/hijera/foxxycode-agent/internal/project"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/version"
)

// CommandDeps wires foxxycode main helpers into the http subcommand without an import cycle.
type CommandDeps struct {
	NewServerRef func(**acp.Server, *config.Config, func() *config.Config) acp.UpdateSender
	EnsureHome   func(string) error
	OpenStore    func(string, *config.Config) (*session.FileStore, error)
}

// StartParams configures PrepareHTTP / StartHTTP.
type StartParams struct {
	CLI              config.CLIPaths
	SessionsRoot     string
	SessionID        string
	ListenAddr       string
	Host             string
	Port             string
	LoggerOverrides  config.LoggerCLIOverrides
	SchedulerEnabled bool
	// AuthToken is the optional bearer token from --auth-token; empty falls back to
	// FOXXYCODE_HTTP_TOKEN and then httpserver.auth_token.
	AuthToken string
	// FolderPicker opens a native folder dialog (desktop mode); nil keeps
	// POST /foxxycode/project/pick-folder at 501.
	FolderPicker FolderPickerFunc
}

// StartedHTTP holds a running HTTP stack built by StartHTTP.
type StartedHTTP struct {
	Server     *Server
	Manager    *session.Manager
	Log        *slog.Logger
	LogCloser  func() error
	ListenAddr string
	Paths      config.Paths
	Config     *config.Config
	httpSrv    *http.Server
}

// StartHTTP resolves config, starts optional scheduler, and builds the HTTP server wrapper.
func StartHTTP(deps CommandDeps, params StartParams) (*StartedHTTP, error) {
	if deps.NewServerRef == nil || deps.EnsureHome == nil || deps.OpenStore == nil {
		return nil, fmt.Errorf("httpserver: incomplete CommandDeps")
	}

	paths, err := config.Resolve(params.CLI)
	if err != nil {
		return nil, err
	}
	if err := deps.EnsureHome(paths.Home); err != nil {
		return nil, err
	}

	cfg, err := config.LoadFromCLI(params.CLI)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if params.SchedulerEnabled {
		cfg.Scheduler.Enabled = true
	}
	if err := cfg.Scheduler.Validate(cfg); err != nil {
		return nil, fmt.Errorf("scheduler: %w", err)
	}

	cfg.Logger.ApplyOverrides(params.LoggerOverrides)
	log, logCloser, err := logger.New(cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}

	log.Info("starting HTTP server", "version", version.Get(), "config", paths.ConfigPath, "workspace", paths.CWD)

	if cfg.SchedulerEffectiveEnabled() {
		scheduler.Start(context.Background(), cfg, log, paths.CWD)
	}

	store, err := deps.OpenStore(params.SessionsRoot, cfg)
	if err != nil {
		_ = logCloser.Close()
		return nil, err
	}
	log.Info("session persistence enabled", "root", store.Root)

	var srv *acp.Server
	var mgr *session.Manager
	live := func() *config.Config {
		if mgr != nil {
			return mgr.Cfg()
		}
		return cfg
	}
	ref := deps.NewServerRef(&srv, cfg, live)
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, snd acp.UpdateSender) (string, error) {
		c := live()
		loop := agent.NewAgent(c, st, snd, log)
		return loop.Run(ctx, prompt)
	}
	mgr = session.NewManager(cfg, ref, runner, log, paths.CWD, store)
	if pid := strings.TrimSpace(params.SessionID); pid != "" {
		if err := session.ValidateFolderSessionID(pid); err != nil {
			_ = logCloser.Close()
			return nil, fmt.Errorf("--session-id: %w", err)
		}
		mgr.SetPreferredSessionID(pid)
	}

	listenAddr := strings.TrimSpace(params.ListenAddr)
	if listenAddr == "" {
		hostStr := strings.TrimSpace(params.Host)
		portStr := strings.TrimSpace(params.Port)
		listenAddr = net.JoinHostPort(hostStr, portStr)
		if hostStr == "0.0.0.0" && portStr == "12345" {
			listenAddr = net.JoinHostPort(cfg.HTTPServer.DefaultListenHost(), cfg.HTTPServer.DefaultListenPortString())
		}
	}

	s := New(cfg, mgr, log, paths.CWD)

	// Out-of-band bearer tokens (--auth-token, then FOXXYCODE_HTTP_TOKEN) enable auth without
	// storing the secret in config.yaml; they union with httpserver.auth_token.
	var extraTokens []string
	if t := strings.TrimSpace(params.AuthToken); t != "" {
		extraTokens = append(extraTokens, t)
	}
	if t := strings.TrimSpace(os.Getenv("FOXXYCODE_HTTP_TOKEN")); t != "" {
		extraTokens = append(extraTokens, t)
	}
	s.SetExtraAuthTokens(extraTokens)
	authOn := len(cfg.HTTPServer.EffectiveAuthTokens()) > 0 || len(extraTokens) > 0
	if effHost, _, err := net.SplitHostPort(listenAddr); err == nil {
		if !authOn && !cfg.HTTPServer.AllowInsecure && !isLoopbackHost(effHost) {
			log.Warn("HTTP API is reachable without authentication",
				"host", effHost,
				"hint", "set httpserver.auth_token / --auth-token / FOXXYCODE_HTTP_TOKEN, or httpserver.allow_insecure: true to silence")
		}
	}

	if ps, err := project.Open(paths.Home); err != nil {
		log.Warn("project store unavailable", "error", err)
	} else {
		// An explicit -cwd flag or FOXXYCODE_CWD wins over the persisted
		// project; otherwise the last opened project is restored.
		explicitCWD := strings.TrimSpace(params.CLI.CWD) != "" ||
			strings.TrimSpace(os.Getenv(config.EnvFOXXYCODECWD)) != ""
		if explicitCWD {
			if err := ps.SetCurrent(paths.CWD); err != nil {
				log.Warn("project seed from cwd", "cwd", paths.CWD, "error", err)
			}
		}
		s.AttachProjectStore(ps)
	}
	s.SetFolderPicker(params.FolderPicker)
	httpSrv := &http.Server{Addr: listenAddr, Handler: s.Handler()}

	return &StartedHTTP{
		Server:     s,
		Manager:    mgr,
		Log:        log,
		LogCloser:  func() error { return logCloser.Close() },
		ListenAddr: listenAddr,
		Paths:      paths,
		Config:     cfg,
		httpSrv:    httpSrv,
	}, nil
}

// ListenAndServe blocks until the HTTP server stops.
func (st *StartedHTTP) ListenAndServe() error {
	st.Log.Info("listening", "addr", st.ListenAddr)
	return st.httpSrv.ListenAndServe()
}

// Serve starts listening in a background goroutine. Returns when the listener is ready or ctx is done.
func (st *StartedHTTP) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", st.ListenAddr)
	if err != nil {
		return err
	}
	st.ListenAddr = ln.Addr().String()
	errCh := make(chan error, 1)
	go func() {
		errCh <- st.httpSrv.Serve(ln)
	}()
	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case err := <-errCh:
			return err
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+st.ListenAddr+"/v1/models", nil)
		if err == nil {
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = res.Body.Close()
				if res.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("http server not ready on %s", st.ListenAddr)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Shutdown gracefully stops the HTTP server and drains background work.
func (st *StartedHTTP) Shutdown(ctx context.Context) error {
	if st.httpSrv != nil {
		_ = st.httpSrv.Shutdown(ctx)
	}
	if st.Server != nil {
		st.Server.Drain()
	}
	if st.LogCloser != nil {
		return st.LogCloser()
	}
	return nil
}

// Run executes the foxxycode http subcommand.
func Run(args []string, deps CommandDeps) error {
	fs := flag.NewFlagSet("http", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfgPath := fs.String("config", "", "path to config.yaml (FOXXYCODE_CONFIG, else <home>/config.yaml or legacy search paths)")
	logLevel := fs.String("log-level", "", "debug|info|warn|error (default from config)")
	logOutput := fs.String("log-output", "", "stdout|stderr|file|both (default from config)")
	logFile := fs.String("log-file", "", "log file path when output includes file (default from config)")
	logFormat := fs.String("log-format", "", "text|json (default from config)")
	homeDir := fs.String("home", "", "agent state directory (FOXXYCODE_HOME, default ~/.foxxycode)")
	httpCWD := fs.String("cwd", "", "default session cwd when the client omits cwd (FOXXYCODE_CWD, default process cwd)")
	sessionsRoot := fs.String("sessions-dir", "", "sessions root (empty uses config sessions.dir or ~/.foxxycode/sessions)")
	persistedSession := fs.String("session-id", "", "optional session id for new sessions (folder name)")
	host := fs.String("H", "0.0.0.0", "bind address for HTTP")
	port := fs.String("P", "12345", "listen port for HTTP")
	fs.StringVar(host, "host", "0.0.0.0", "bind address for HTTP (alias of -H)")
	fs.StringVar(port, "port", "12345", "listen port (alias of -P)")
	schedulerEnabled := fs.Bool("scheduler-enabled", false, "set scheduler.enabled=true in this process (build with -tags scheduler)")
	authToken := fs.String("auth-token", "", "bearer token required on /v1/* and /foxxycode/* routes (else FOXXYCODE_HTTP_TOKEN, else httpserver.auth_token). Empty = no auth")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage of http:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	st, err := StartHTTP(deps, StartParams{
		CLI: config.CLIPaths{
			Home:   strings.TrimSpace(*homeDir),
			CWD:    strings.TrimSpace(*httpCWD),
			Config: strings.TrimSpace(*cfgPath),
		},
		SessionsRoot: *sessionsRoot,
		SessionID:    *persistedSession,
		Host:         *host,
		Port:         *port,
		LoggerOverrides: config.LoggerCLIOverrides{
			Level:  strings.TrimSpace(*logLevel),
			Output: strings.TrimSpace(*logOutput),
			File:   strings.TrimSpace(*logFile),
			Format: strings.TrimSpace(*logFormat),
		},
		SchedulerEnabled: *schedulerEnabled,
		AuthToken:        strings.TrimSpace(*authToken),
	})
	if err != nil {
		return err
	}
	defer func() { _ = st.Shutdown(context.Background()) }()
	return st.ListenAndServe()
}

// isLoopbackHost reports whether a bind host only accepts local connections.
func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "", "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
