//go:build gateway || gateway.telegram

package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// ProjectID keeps a project's Telegram sessions separate from other IDE projects.
func ProjectID(home, cwd string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(home) + "\x00" + filepath.Clean(cwd)))
	return hex.EncodeToString(sum[:16])
}

// Client implements the Telegram library's HTTPClient through the local relay.
// The original request URL is never dialed; only the relay knows the bot's origin.
type Client struct {
	reg      Registration
	dir      string
	http     *http.Client
	mu       sync.Mutex
	endpoint discovery
	closed   bool
	cancel   context.CancelFunc
	done     chan struct{}
}

// Connect registers a backend and keeps its lease alive, including while idle.
func Connect(ctx context.Context, reg Registration) (*Client, error) {
	dir, err := relayDirectory()
	if err != nil {
		return nil, err
	}
	return connect(ctx, dir, reg)
}

func connect(ctx context.Context, dir string, reg Registration) (*Client, error) {
	reg.Config.Token = reg.Config.EffectiveToken()
	// Resolve inheritance in this editor's environment. The shared worker may
	// have been launched by another IDE with a different system/IDE proxy.
	mode, _, err := config.ParseProxySetting(reg.Config.Proxy)
	if err != nil {
		return nil, err
	}
	if mode == config.ProxyModeInherit {
		origin := strings.TrimSpace(reg.APIBase)
		if origin == "" {
			origin = "https://api.telegram.org"
		}
		req, err := http.NewRequest(http.MethodGet, origin, nil)
		if err != nil {
			return nil, errors.New("invalid Bot API origin")
		}
		proxy, err := http.ProxyFromEnvironment(req)
		if err != nil {
			return nil, errors.New("invalid inherited Telegram proxy")
		}
		reg.Config.Proxy = config.ProxyNone
		if proxy != nil {
			reg.Config.Proxy = proxy.String()
		}
	}
	reg.ID = randomID()
	c := &Client{reg: reg, dir: dir, http: localClient(), done: make(chan struct{})}
	if err := c.register(ctx); err != nil {
		c.http.CloseIdleConnections()
		return nil, err
	}
	ctx, c.cancel = context.WithCancel(ctx)
	go func() {
		defer close(c.done)
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				_ = c.register(ctx)
			}
		}
	}()
	return c, nil
}

func (c *Client) register(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("telegram relay client closed")
	}
	d, err := ensure(ctx, c.dir)
	if err != nil {
		return err
	}
	data, err := json.Marshal(c.reg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL+"/register", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.Secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return errors.New("telegram relay registration failed")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		message, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return errors.New("telegram relay: " + strings.TrimSpace(string(message)))
	}
	c.endpoint = d
	return nil
}

// Do forwards all Bot API methods. A failed mutation is never replayed automatically.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	d, closed := c.endpoint, c.closed
	c.mu.Unlock()
	if closed {
		return nil, errors.New("telegram relay client closed")
	}
	method := path.Base(req.URL.Path)
	forward, err := http.NewRequestWithContext(req.Context(), http.MethodPost, d.URL+"/api/"+c.reg.ID+"/"+method, req.Body)
	if err != nil {
		return nil, err
	}
	forward.Header = req.Header.Clone()
	forward.Header.Set("Authorization", "Bearer "+d.Secret)
	res, err := c.http.Do(forward)
	if err != nil {
		// The heartbeat handles a crashed relay; reconnect immediately as well so
		// the library's next poll does not wait for the heartbeat interval.
		_ = c.register(req.Context())
		return nil, errors.New("telegram relay unavailable; reconnecting")
	}
	if res.StatusCode == http.StatusGone {
		_ = c.register(req.Context())
	}
	return res, nil
}

// Close unregisters this backend. Other editor projects keep their connection.
func (c *Client) Close() {
	c.cancel()
	<-c.done
	c.mu.Lock()
	c.closed = true
	d := c.endpoint
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, d.URL+"/clients/"+c.reg.ID, nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+d.Secret)
		if res, err := c.http.Do(req); err == nil {
			_ = res.Body.Close()
		}
	}
	c.http.CloseIdleConnections()
}
