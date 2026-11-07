//go:build gateway || gateway.telegram

package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/platform"
)

// WorkerEnv selects the private worker entrypoint, before normal command parsing.
// The value is the user-private discovery directory, never a token or a command.
const WorkerEnv = "FOXXYCODE_INTERNAL_TELEGRAM_RELAY"

type discovery struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
	PID    int    `json:"pid"`
}

func randomID() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("system randomness unavailable")
	}
	return hex.EncodeToString(value[:])
}

func relayDirectory() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "FoxxyCode", "telegram-relay-v1"), nil
}

func localClient() *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 35 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

// ensure serializes discovery and startup across both editor families and all homes.
// A stale record is harmless: only an authenticated, live loopback endpoint is reused.
func ensure(ctx context.Context, dir string) (discovery, error) {
	if err := ctx.Err(); err != nil {
		return discovery{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return discovery{}, err
	}
	unlock, err := platform.LockFile(filepath.Join(dir, "launch.lock"))
	if err != nil {
		return discovery{}, err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return discovery{}, err
	}
	if d, ok := readLive(ctx, dir); ok {
		return d, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return discovery{}, err
	}
	cmd := exec.Command(exe) // #nosec G204 -- this binary's private worker, no shell
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(name, WorkerEnv) {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, WorkerEnv+"="+dir)
	platform.DetachFromTerminal(cmd)
	platform.HideConsoleWindow(cmd)
	platform.AdaptCommand(cmd)
	if err := cmd.Start(); err != nil {
		return discovery{}, errors.New("cannot start Telegram relay worker")
	}
	// Wait reaps the process without tying its lifetime to the launching backend.
	go func() { _ = cmd.Wait() }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if d, ok := readLive(ctx, dir); ok {
			return d, nil
		}
		select {
		case <-ctx.Done():
			return discovery{}, ctx.Err()
		case <-timer.C:
			return discovery{}, errors.New("telegram relay did not become ready")
		case <-tick.C:
		}
	}
}

func readLive(ctx context.Context, dir string) (discovery, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "endpoint.json")) // #nosec G304 -- private user cache
	var d discovery
	if err != nil || json.Unmarshal(data, &d) != nil || len(d.Secret) != 64 {
		return d, false
	}
	origin, err := url.Parse(d.URL)
	if err != nil || origin.Scheme != "http" || origin.Hostname() != "127.0.0.1" || origin.Port() == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return d, false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, d.URL+"/health", nil)
	if err != nil {
		return d, false
	}
	req.Header.Set("Authorization", "Bearer "+d.Secret)
	hc := localClient()
	defer hc.CloseIdleConnections()
	res, err := hc.Do(req)
	if err != nil {
		return d, false
	}
	_ = res.Body.Close()
	return d, res.StatusCode == http.StatusOK
}

// RunWorker serves one user-wide relay, stopping after the last editor lease expires.
func RunWorker(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	unlock, err := platform.LockFile(filepath.Join(dir, "worker.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	// Another launcher may have started us just as an earlier worker became ready.
	if _, ok := readLive(ctx, dir); ok {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d := discovery{URL: "http://" + ln.Addr().String(), Secret: randomID(), PID: os.Getpid()}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "endpoint.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()
	s := NewServer(ctx, d.Secret)
	s.loadSelections(filepath.Join(dir, "selection.json"))
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 40 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return srv.Close()
		case now := <-tick.C:
			if s.Sweep(now) {
				cancel()
				return srv.Close()
			}
		}
	}
}
