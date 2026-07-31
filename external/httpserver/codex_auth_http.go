//go:build http

package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type codexAuthLoginAttempt struct {
	ProviderName string
	Status       string
	Connected    bool
	Error        string
	CreatedAt    time.Time
	// cancel stops the waiting goroutine when the server drains.
	cancel context.CancelFunc
}

type codexAuthLoginResponse struct {
	LoginID         string `json:"login_id,omitempty"`
	VerificationURL string `json:"verification_url,omitempty"`
	UserCode        string `json:"user_code,omitempty"`
	Status          string `json:"status,omitempty"`
	Connected       bool   `json:"connected"`
	Error           string `json:"error,omitempty"`
}

// cancelCodexAuthLogins stops every sign-in still waiting for confirmation.
// Called from Drain so shutdown does not wait out the device-flow timeout.
func (s *Server) cancelCodexAuthLogins() {
	s.codexAuthMu.Lock()
	defer s.codexAuthMu.Unlock()
	for _, attempt := range s.codexAuthLogins {
		if attempt.cancel != nil {
			attempt.cancel()
		}
	}
}

func (s *Server) registerCodexAuthRoutes() {
	s.mux.HandleFunc("GET /foxxycode/providers/{name}/codex-auth", s.foxxycodeProviderCodexAuthGet)
	s.mux.HandleFunc("DELETE /foxxycode/providers/{name}/codex-auth", s.foxxycodeProviderCodexAuthDelete)
	s.mux.HandleFunc("POST /foxxycode/providers/{name}/codex-auth/device", s.foxxycodeProviderCodexAuthDevicePost)
	s.mux.HandleFunc("GET /foxxycode/providers/{name}/codex-auth/device/{loginID}", s.foxxycodeProviderCodexAuthDeviceGet)
}

func (s *Server) foxxycodeProviderCodexAuthGet(w http.ResponseWriter, r *http.Request) {
	name, _, ok := s.resolveCodexAuthProvider(w, r.PathValue("name"))
	if !ok {
		return
	}
	status, err := llm.InspectCodexAuth(config.CodexAuthPath(s.activeCfg().Paths.Home, name))
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCodexAuthJSON(w, http.StatusOK, status)
}

func (s *Server) foxxycodeProviderCodexAuthDelete(w http.ResponseWriter, r *http.Request) {
	name, _, ok := s.resolveCodexAuthProvider(w, r.PathValue("name"))
	if !ok {
		return
	}
	path := config.CodexAuthPath(s.activeCfg().Paths.Home, name)
	if err := llm.RemoveCodexAuth(path); err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "could not remove Codex credentials")
		return
	}
	status, err := llm.InspectCodexAuth(path)
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCodexAuthJSON(w, http.StatusOK, status)
}

func (s *Server) foxxycodeProviderCodexAuthDevicePost(w http.ResponseWriter, r *http.Request) {
	name, provider, ok := s.resolveCodexAuthProvider(w, r.PathValue("name"))
	if !ok {
		return
	}
	client, err := llm.HTTPClientForOptionalProxy(provider.Proxy)
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, err.Error())
		return
	}
	login, err := llm.StartCodexDeviceLogin(r.Context(), s.codexAuthIssuer, client)
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadGateway, err.Error())
		return
	}
	loginID := newCodexAuthLoginID()
	// The wait outlives this request but not the server: Drain cancels it, so a
	// sign-in nobody confirms cannot keep polling or write into a home directory
	// the caller has already torn down.
	waitCtx, cancel := context.WithCancel(context.Background())
	attempt := &codexAuthLoginAttempt{ProviderName: name, Status: "pending", CreatedAt: time.Now(), cancel: cancel}
	s.codexAuthMu.Lock()
	for id, old := range s.codexAuthLogins {
		if time.Since(old.CreatedAt) > 20*time.Minute {
			if old.cancel != nil {
				old.cancel()
			}
			delete(s.codexAuthLogins, id)
		}
	}
	s.codexAuthLogins[loginID] = attempt
	s.codexAuthMu.Unlock()

	authPath := config.CodexAuthPath(s.activeCfg().Paths.Home, name)
	issuer := s.codexAuthIssuer
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		defer cancel()
		err := llm.CompleteCodexDeviceLogin(waitCtx, issuer, client, login, authPath)
		s.codexAuthMu.Lock()
		defer s.codexAuthMu.Unlock()
		if err != nil {
			attempt.Status = "failed"
			attempt.Error = err.Error()
			return
		}
		attempt.Status = "completed"
		attempt.Connected = true
	}()

	writeCodexAuthJSON(w, http.StatusOK, codexAuthLoginResponse{
		LoginID:         loginID,
		VerificationURL: login.VerificationURL,
		UserCode:        login.UserCode,
		Status:          "pending",
	})
}

func (s *Server) foxxycodeProviderCodexAuthDeviceGet(w http.ResponseWriter, r *http.Request) {
	name, _, ok := s.resolveCodexAuthProvider(w, r.PathValue("name"))
	if !ok {
		return
	}
	s.codexAuthMu.Lock()
	attempt := s.codexAuthLogins[r.PathValue("loginID")]
	if attempt == nil || attempt.ProviderName != name {
		s.codexAuthMu.Unlock()
		writeFoxxyCodeConfigErr(w, http.StatusNotFound, "unknown Codex login")
		return
	}
	response := codexAuthLoginResponse{
		Status:    attempt.Status,
		Connected: attempt.Connected,
		Error:     attempt.Error,
	}
	s.codexAuthMu.Unlock()
	writeCodexAuthJSON(w, http.StatusOK, response)
}

// resolveCodexAuthProvider accepts saved Codex providers and valid unsaved names
// so a newly added provider can be signed in before the settings document is saved.
func (s *Server) resolveCodexAuthProvider(w http.ResponseWriter, rawName string) (string, config.ProviderConfig, bool) {
	c := s.activeCfg()
	if c == nil || strings.TrimSpace(c.Paths.Home) == "" {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config home unavailable")
		return "", config.ProviderConfig{}, false
	}
	name := strings.TrimSpace(rawName)
	probe := config.ProviderConfig{Name: name, Type: "codex"}
	probe.Normalize()
	if err := probe.Validate(); err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, err.Error())
		return "", config.ProviderConfig{}, false
	}
	if saved := c.FindProvider(name); saved != nil {
		if saved.Type != "codex" {
			writeFoxxyCodeConfigErr(w, http.StatusConflict, "provider is not a Codex provider")
			return "", config.ProviderConfig{}, false
		}
		return name, *saved, true
	}
	return name, probe, true
}

func newCodexAuthLoginID() string {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func writeCodexAuthJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
