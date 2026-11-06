// Package autocomplete completes editor text independently of HTTP and the agent loop.
package autocomplete

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

var (
	ErrConfigUnavailable = errors.New("config unavailable")
	ErrModelUnavailable  = errors.New("LLM unavailable")
)

// Request describes the caret and its neighboring text.
type Request struct {
	Prefix   string `json:"prefix"`
	Suffix   string `json:"suffix"`
	Path     string `json:"path"`
	Language string `json:"language"`
	Debug    bool   `json:"debug"`
}

// Options supplies dependencies for one request; none are stored process-wide.
type Options struct {
	Log             *slog.Logger
	ProviderFactory func(llm.ProviderInput) (llm.Provider, error)
	RelatedFiles    func(current string, limit int) []llm.FIMFile
	DisplayPath     func(string) string
}

// Result preserves the completion and optional diagnostic fields.
type Result struct {
	Completion, Model, Mode string
	Enabled                 bool
	Extra                   map[string]interface{}
}

// Service owns per-host statistics and model capabilities. Its zero value is ready.
type Service struct{ state state }

// Snapshot returns completion statistics and editor feedback counters.
func (s *Service) Snapshot() map[string]interface{} { return s.state.snapshot() }

// Feedback records one editor outcome, rejecting unknown events.
func (s *Service) Feedback(event string) bool { return s.state.feedback(event) }

// Complete fills one caret position. Cancellation, filtering and fallback are
// owned here; transports only map the returned result or error to their protocol.
func (svc *Service) Complete(parent context.Context, cfg *config.Config, body Request, opts Options) (Result, error) {
	if cfg == nil {
		return Result{}, ErrConfigUnavailable
	}
	logger := opts.Log
	if logger == nil {
		logger = slog.Default()
	}
	s := runner{autocomplete: &svc.state, log: logger, agentProviderFactory: opts.ProviderFactory}
	ac := cfg.Autocomplete
	ac.ApplyDefaults()
	if !ac.AutocompleteEnabled() {
		return Result{}, nil
	}
	result := Result{Enabled: true}
	prefix, suffix := tailBytes(body.Prefix, ac.MaxPrefixBytes), headBytes(body.Suffix, ac.MaxSuffixBytes)
	if (strings.TrimSpace(prefix) == "" && strings.TrimSpace(suffix) == "") || startsMidIdentifier(suffix) {
		return result, nil
	}
	modelID, err := autocompleteModelID(cfg, &ac)
	if err != nil {
		s.log.Error("autocomplete provider", "error", err)
		return Result{}, fmt.Errorf("%w: %w", ErrModelUnavailable, err)
	}
	result.Model = modelID
	multiLine := ac.MultiLineEnabled() && decideMultiLine(prefix, suffix)
	stops := lineStops(multiLine, suffix)
	var files []llm.FIMFile
	if opts.RelatedFiles != nil {
		files = opts.RelatedFiles(body.Path, ac.RelatedFileCount())
	}
	displayPath := body.Path
	if opts.DisplayPath != nil {
		displayPath = opts.DisplayPath(body.Path)
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(ac.TimeoutMS)*time.Millisecond)
	defer cancel()
	s.autocomplete.requests.Add(1)
	started := time.Now()
	text, mode, resp, err := s.completeHole(ctx, cfg, &ac, modelID, holeRequest{
		path: displayPath, language: body.Language, prefix: prefix, suffix: suffix,
		files: files, multiLine: multiLine, stops: stops,
	})
	result.Mode = mode
	if err != nil {
		switch {
		case parent.Err() != nil:
			s.autocomplete.cancelled.Add(1)
			return Result{}, parent.Err()
		case ctx.Err() != nil || isTimeoutError(err):
			s.autocomplete.timeouts.Add(1)
			s.log.Warn("autocomplete: model did not answer within autocomplete.timeout_ms", "model", modelID, "mode", mode, "timeout_ms", ac.TimeoutMS)
			result.Extra = map[string]interface{}{"timed_out": true}
			return result, nil
		case llm.HTTPStatus(err) == 429:
			s.autocomplete.rateLimited.Add(1)
			delay := RateLimitPause
			if hint, ok := llm.RetryDelayHint(err); ok && hint > 0 {
				delay = hint
			}
			s.log.Warn("autocomplete: provider rate limit", "model", modelID, "retry_after", delay, "error", err)
		default:
			s.autocomplete.errors.Add(1)
			s.log.Error("autocomplete llm", "model", modelID, "mode", mode, "error", err)
		}
		return Result{}, err
	}
	s.autocomplete.recordLatency(time.Since(started).Milliseconds())
	if resp != nil {
		s.autocomplete.promptTok.Add(int64(resp.InputTokens))
		s.autocomplete.outputTok.Add(int64(resp.OutputTokens))
	}
	result.Completion = cleanCompletion(text, prefix, suffix, multiLine)
	if result.Completion == "" {
		s.autocomplete.empty.Add(1)
	} else {
		s.autocomplete.served.Add(1)
	}
	s.log.Debug("autocomplete", "model", modelID, "mode", mode, "multi_line", multiLine,
		"related_files", len(files), "latency_ms", time.Since(started).Milliseconds(), "chars", len(result.Completion), "raw", text)
	if body.Debug {
		result.Extra = map[string]interface{}{"raw": text, "multi_line": multiLine, "stops": stops}
	}
	return result, nil
}
