//go:build http

package httpserver

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/autocomplete"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// foxxycodeCompletionConfigGet serves the handful of settings an editor client needs before it can
// behave: whether to run at all, when to ask, and how much context to send. Clients read this
// instead of the whole config document so the knobs stay in one place (config.autocomplete).
func (s *Server) foxxycodeCompletionConfigGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	ac := config.AutocompleteConfig{}
	if cfg := s.activeCfg(); cfg != nil {
		ac = cfg.Autocomplete
	}
	ac.ApplyDefaults()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":          ac.AutocompleteEnabled(),
		"trigger":          ac.Trigger,
		"debounce_ms":      ac.DebounceMS,
		"multi_line":       ac.MultiLineEnabled(),
		"timeout_ms":       ac.TimeoutMS,
		"max_prefix_bytes": ac.MaxPrefixBytes,
		"max_suffix_bytes": ac.MaxSuffixBytes,
	})
}

// foxxycodeCompletionStatsGet reports the counters behind autocomplete.Service, the numbers that say
// whether inline completion is fast enough and accepted often enough to keep.
func (s *Server) foxxycodeCompletionStatsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.autocomplete.Snapshot())
}

// foxxycodeCompletionFeedbackPost takes one editor-reported outcome (shown, accepted, dismissed,
// cache_hit). Fire-and-forget from the clients; it only moves a counter.
func (s *Server) foxxycodeCompletionFeedbackPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Event string `json:"event"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	if !s.autocomplete.Feedback(strings.ToLower(strings.TrimSpace(body.Event))) {
		http.Error(w, `{"error":{"message":"event must be shown, accepted, dismissed or cache_hit"}}`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// foxxycodeCompletionPost adapts the standalone completion service to HTTP.
func (s *Server) foxxycodeCompletionPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body autocomplete.Request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	result, err := s.autocomplete.Complete(r.Context(), s.activeCfg(), body, autocomplete.Options{
		Log: s.log, ProviderFactory: s.agentProviderFactory,
		RelatedFiles: func(current string, limit int) []llm.FIMFile {
			return autocomplete.RelatedFiles(s.defaultCWD, current, limit, ideenv.Get().OpenFiles)
		},
		DisplayPath: func(path string) string { return autocomplete.DisplayPath(s.defaultCWD, path) },
	})
	if err != nil {
		switch {
		case r.Context().Err() != nil:
			return
		case errors.Is(err, autocomplete.ErrConfigUnavailable):
			http.Error(w, `{"error":{"message":"config unavailable"}}`, http.StatusServiceUnavailable)
		case errors.Is(err, autocomplete.ErrModelUnavailable):
			http.Error(w, `{"error":{"message":"LLM unavailable"}}`, http.StatusServiceUnavailable)
		case llm.HTTPStatus(err) == http.StatusTooManyRequests:
			delay := autocomplete.RateLimitPause
			if d, ok := llm.RetryDelayHint(err); ok && d > 0 {
				delay = d
			}
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(delay.Seconds()))))
			http.Error(w, `{"error":{"message":"rate limited by the model provider"}}`, http.StatusTooManyRequests)
		default:
			http.Error(w, `{"error":{"message":"LLM error"}}`, http.StatusBadGateway)
		}
		return
	}
	writeCompletion(w, result.Completion, result.Model, result.Mode, result.Enabled, result.Extra)
}

func writeCompletion(w http.ResponseWriter, completion, model, mode string, enabled bool, extra map[string]interface{}) {
	out := map[string]interface{}{
		"completion": completion,
		"model":      model,
		"mode":       mode,
		"enabled":    enabled,
	}
	for k, v := range extra {
		out[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
