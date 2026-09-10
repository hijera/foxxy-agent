//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/textenc"
)

// autocompleteInstruction drives the chat-mode fill-in-the-middle pass. Unlike
// enhancePromptInstruction it is deliberately not wrapped in prompts.WithIdentity: the identity
// preamble would be re-sent on every keystroke, and the model is not acting as the assistant
// here - it is completing text. The two examples do more than the rules: chat models follow a
// demonstrated shape far more reliably than a prohibition.
const autocompleteInstruction = "You complete code inline in an editor, like a code completion engine. " +
	"The user message holds part of a file with the caret marked <CURSOR>, sometimes preceded by excerpts of related files. " +
	"Reply with the text to insert at <CURSOR> and nothing else: no explanation, no markdown fences, " +
	"and no repetition of the text before or after the caret. " +
	"Keep the insertion short - finish the current line, or the small block the caret has just opened. " +
	"Match the surrounding indentation and code style. " +
	"If nothing useful can be inserted, reply with nothing at all.\n\n" +
	"Example 1\nInput:\ndef area(r):\n    return <CURSOR>\nOutput:\n3.14159 * r * r\n\n" +
	"Example 2\nInput:\nfor (const item of items) {<CURSOR>\n}\nOutput:\n\n  if (!item) continue;\n  console.log(item);\n\n" +
	"Example 3\nInput:\nfunc abs(x int) int {<CURSOR>\n}\nOutput:\n\n\tif x < 0 {\n\t\treturn -x\n\t}\n\treturn x"

// autocompleteCursor marks the caret position in the chat prompt.
const autocompleteCursor = "<CURSOR>"

// Neighbouring-file excerpts: enough for imports and the first signatures, never a whole file.
const (
	relatedFileLines     = 40
	relatedFileBytes     = 1500
	relatedFileReadBytes = 16 * 1024
)

// autocompleteRateLimitPause is the Retry-After sent to clients when the provider rate-limits a
// suggestion but names no pause of its own.
const autocompleteRateLimitPause = 10 * time.Second

// autocompleteThinkingBudget is the token floor for a model known to reason before answering.
// Its reasoning alone runs past a normal suggestion budget, so keeping autocomplete.max_tokens
// would pay for the thinking and get nothing back; the floor buys the answer. The model entry's
// own max_tokens still caps it.
const autocompleteThinkingBudget = 1024

// thinkingBudget returns a copy of ac with max_tokens raised to autocompleteThinkingBudget.
func thinkingBudget(ac *config.AutocompleteConfig) *config.AutocompleteConfig {
	c := *ac
	if c.MaxTokens < autocompleteThinkingBudget {
		c.MaxTokens = autocompleteThinkingBudget
	}
	return &c
}

// isTimeoutError recognises a model call that ran out of time on our side, whether the request
// context or the HTTP client's own deadline fired first.
func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "Client.Timeout")
}

type autocompleteRequestBody struct {
	Prefix   string `json:"prefix"`
	Suffix   string `json:"suffix"`
	Path     string `json:"path"`
	Language string `json:"language"`
	// Debug asks for the model's text before cleaning in the reply (raw), so a quality harness
	// can tell a bad model answer from an over-eager filter.
	Debug bool `json:"debug"`
}

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

// foxxycodeCompletionStatsGet reports the counters behind autocompleteState, the numbers that say
// whether inline completion is fast enough and accepted often enough to keep.
func (s *Server) foxxycodeCompletionStatsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.autocomplete.snapshot())
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
	if !s.autocomplete.feedback(strings.ToLower(strings.TrimSpace(body.Event))) {
		http.Error(w, `{"error":{"message":"event must be shown, accepted, dismissed or cache_hit"}}`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// foxxycodeCompletionPost returns one inline suggestion for the caret position described by the
// request. It is a single LLM call with no tools, no session and no agent loop - the user is
// typing, not conversing. The request context is the only cancellation needed: when the user
// types again the client drops the connection and the upstream call dies with it.
func (s *Server) foxxycodeCompletionPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	var body autocompleteRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}

	cfg := s.activeCfg()
	if cfg == nil {
		http.Error(w, `{"error":{"message":"config unavailable"}}`, http.StatusServiceUnavailable)
		return
	}
	ac := cfg.Autocomplete
	ac.ApplyDefaults()

	// A disabled section answers 200 with enabled:false rather than an error: the client should
	// stop asking, not treat it as a failure and retry.
	if !ac.AutocompleteEnabled() {
		writeCompletion(w, "", "", "", false, nil)
		return
	}

	prefix := tailBytes(body.Prefix, ac.MaxPrefixBytes)
	suffix := headBytes(body.Suffix, ac.MaxSuffixBytes)
	// Nothing to complete, or the caret is inside a word the user is still typing: both would
	// only buy a wrong guess.
	if (strings.TrimSpace(prefix) == "" && strings.TrimSpace(suffix) == "") || startsMidIdentifier(suffix) {
		writeCompletion(w, "", "", "", true, nil)
		return
	}

	modelID, err := autocompleteModelID(cfg, &ac)
	if err != nil {
		s.log.Error("autocomplete provider", "error", err)
		http.Error(w, `{"error":{"message":"LLM unavailable"}}`, http.StatusServiceUnavailable)
		return
	}

	multiLine := ac.MultiLineEnabled() && decideMultiLine(prefix, suffix)
	stops := lineStops(multiLine, suffix)
	files := s.relatedFiles(body.Path, ac.RelatedFileCount())
	displayPath := s.displayPath(body.Path)

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(ac.TimeoutMS)*time.Millisecond)
	defer cancel()
	s.autocomplete.requests.Add(1)
	started := time.Now()

	text, mode, resp, err := s.completeHole(ctx, cfg, &ac, modelID, holeRequest{
		path: displayPath, language: body.Language, prefix: prefix, suffix: suffix,
		files: files, multiLine: multiLine, stops: stops,
	})
	if err != nil {
		switch {
		case r.Context().Err() != nil:
			// The client typed on and dropped the connection: the normal case, not a fault
			// worth logging loudly, and nobody is left to read the response.
			s.autocomplete.cancelled.Add(1)
			return
		case ctx.Err() != nil || isTimeoutError(err):
			// Our own deadline: answer with nothing rather than an error, so the editor treats
			// it as "no suggestion" and keeps going. Counted, because a model that is
			// routinely this slow is the wrong model for the job.
			s.autocomplete.timeouts.Add(1)
			s.log.Warn("autocomplete: model did not answer within autocomplete.timeout_ms", "model", modelID, "mode", mode, "timeout_ms", ac.TimeoutMS)
			writeCompletion(w, "", modelID, mode, true, map[string]interface{}{"timed_out": true})
			return
		case llm.HTTPStatus(err) == http.StatusTooManyRequests:
			// Passed on as a 429 with Retry-After so clients pause their automatic requests
			// instead of hammering a provider that has already said no.
			s.autocomplete.rateLimited.Add(1)
			delay := autocompleteRateLimitPause
			if d, ok := llm.RetryDelayHint(err); ok && d > 0 {
				delay = d
			}
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(delay.Seconds()))))
			s.log.Warn("autocomplete: provider rate limit", "model", modelID, "retry_after", delay, "error", err)
			http.Error(w, `{"error":{"message":"rate limited by the model provider"}}`, http.StatusTooManyRequests)
			return
		}
		s.autocomplete.errors.Add(1)
		s.log.Error("autocomplete llm", "model", modelID, "mode", mode, "error", err)
		http.Error(w, `{"error":{"message":"LLM error"}}`, http.StatusBadGateway)
		return
	}
	s.autocomplete.recordLatency(time.Since(started).Milliseconds())
	if resp != nil {
		s.autocomplete.promptTok.Add(int64(resp.InputTokens))
		s.autocomplete.outputTok.Add(int64(resp.OutputTokens))
	}

	clean := cleanCompletion(text, prefix, suffix, multiLine)
	if clean == "" {
		s.autocomplete.empty.Add(1)
	} else {
		s.autocomplete.served.Add(1)
	}
	s.log.Debug("autocomplete", "model", modelID, "mode", mode, "multi_line", multiLine,
		"related_files", len(files), "latency_ms", time.Since(started).Milliseconds(), "chars", len(clean), "raw", text)
	var extra map[string]interface{}
	if body.Debug {
		extra = map[string]interface{}{"raw": text, "multi_line": multiLine, "stops": stops}
	}
	writeCompletion(w, clean, modelID, mode, true, extra)
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

// holeRequest is everything completeHole needs to fill the gap at the caret.
type holeRequest struct {
	path, language string
	prefix, suffix string
	files          []llm.FIMFile
	multiLine      bool
	stops          []string
}

// completeHole asks the model for the text at the caret, choosing between native
// fill-in-the-middle and a chat prompt per autocomplete.mode. It returns the raw model text, the
// mode that produced it ("fim" or "chat"), and the response for its usage counters (nil when the
// stream was cut short by cutoffReached).
//
// In auto mode a raw FIM call that fails - a gateway without /v1/completions, a model that
// rejects the tokens - is re-issued as chat and the model is remembered as chat-only, so the
// failure is paid for once rather than on every keystroke.
func (s *Server) completeHole(ctx context.Context, cfg *config.Config, ac *config.AutocompleteConfig, modelID string, req holeRequest) (string, string, *llm.Response, error) {
	tmpl, hasTmpl := llm.FIMTemplateFor(modelID)
	useFIM := ac.Mode == config.AutocompleteModeFIM ||
		(ac.Mode == config.AutocompleteModeAuto && hasTmpl && !s.autocomplete.isFIMBroken(modelID))

	if useFIM {
		if !hasTmpl {
			return "", "fim", nil, fmt.Errorf("model %q has no known fill-in-the-middle token convention", modelID)
		}
		provider, err := s.autocompleteProvider(cfg, ac, capStops(append(append([]string{}, req.stops...), tmpl.Stop...), autocompleteMaxStops))
		if err != nil {
			return "", "fim", nil, err
		}
		rc, ok := llm.AsRawCompleter(provider)
		switch {
		case ok:
			resp, err := rc.CompleteRaw(ctx, tmpl.Prompt(req.path, req.prefix, req.suffix, req.files, commentLeader(req.language)))
			switch {
			case err == nil && (strings.TrimSpace(resp.Content) != "" || ac.Mode == config.AutocompleteModeFIM):
				s.autocomplete.fim.Add(1)
				s.autocomplete.noteFIMServed(modelID)
				return resp.Content, "fim", resp, nil
			case err == nil:
				// 200 with nothing in it: the endpoint exists but the model did not fill the
				// hole. One empty answer is retried as chat; a streak retires FIM for the model.
				s.autocomplete.fimEmpty.Add(1)
				if s.autocomplete.noteFIMEmpty(modelID) {
					s.log.Warn("autocomplete: raw fill-in-the-middle keeps returning nothing, using chat prompts for this model from now on", "model", modelID)
				}
			case ctx.Err() != nil || ac.Mode == config.AutocompleteModeFIM:
				return "", "fim", nil, err
			default:
				s.autocomplete.markFIMBroken(modelID)
				s.autocomplete.fimFallback.Add(1)
				s.log.Warn("autocomplete: raw fill-in-the-middle failed, using chat prompts for this model from now on", "model", modelID, "error", err)
			}
		case ac.Mode == config.AutocompleteModeFIM:
			return "", "fim", nil, fmt.Errorf("provider of %q cannot run raw completions; fill-in-the-middle needs an OpenAI-compatible backend", modelID)
		}
	}

	// Stop sequences are matched against the whole generation, reasoning included: for a model
	// that thinks before answering, a "\n" stop fires on the first line of its thoughts and the
	// answer never comes. Such models are remembered and get no stops; the streamed cutoff, which
	// only watches answer text, ends their suggestions instead.
	stops := capStops(req.stops, autocompleteMaxStops)
	budget := ac
	if s.autocomplete.isThinking(modelID) {
		stops = nil
		budget = thinkingBudget(ac)
	}
	provider, err := s.autocompleteProvider(cfg, budget, stops)
	if err != nil {
		return "", "chat", nil, err
	}
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: autocompleteInstruction},
		{Role: llm.RoleUser, Content: chatPrompt(req)},
	}
	out, err := streamWithCutoff(ctx, provider, messages, req.multiLine, caretIndent(req.prefix))
	if err != nil {
		return "", "chat", nil, err
	}
	if out.reasoned && !s.autocomplete.isThinking(modelID) {
		s.autocomplete.markThinking(modelID)
		if strings.TrimSpace(out.text) == "" && len(stops) > 0 {
			// The first request paid for the lesson; re-issue it once without stops rather than
			// hand the editor an empty suggestion.
			s.autocomplete.reasoningRetries.Add(1)
			s.log.Warn("autocomplete: model reasons before answering and the stop sequences cut that short; retrying without them, with a larger budget, and keeping both for this model (a -noreason variant, if the hub has one, is much faster)", "model", modelID, "budget", autocompleteThinkingBudget)
			if provider, err = s.autocompleteProvider(cfg, thinkingBudget(ac), nil); err != nil {
				return "", "chat", nil, err
			}
			if out, err = streamWithCutoff(ctx, provider, messages, req.multiLine, caretIndent(req.prefix)); err != nil {
				return "", "chat", nil, err
			}
		}
	}
	if out.reasoned && strings.TrimSpace(out.text) == "" {
		// A thinking model that used the whole budget on reasoning leaves nothing to show; that
		// is a configuration problem worth one loud line, not a silent "no suggestion".
		s.log.Warn("autocomplete: model spent its budget thinking and returned no text; pick a -noreason model or raise autocomplete.max_tokens", "model", modelID)
	}
	s.autocomplete.chat.Add(1)
	return out.text, "chat", out.resp, nil
}

// streamedAnswer is what streamWithCutoff hands back: the answer text, the provider's response
// when the stream ran to its end (nil when cut short), and whether the model emitted reasoning
// before (or instead of) the answer.
type streamedAnswer struct {
	text     string
	resp     *llm.Response
	reasoned bool
}

// streamWithCutoff runs the chat request as a stream and stops it the moment cutoffReached says
// the suggestion is complete, instead of waiting for the token budget to run out. A stream the
// function cancelled itself is a success carrying the text so far; the response object is then
// nil because the provider never got to finish it. Reasoning deltas are watched but never part
// of the answer: only answer text can trigger the cutoff.
func streamWithCutoff(ctx context.Context, provider llm.Provider, messages []llm.Message, multiLine bool, indent string) (streamedAnswer, error) {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var text strings.Builder
	var cut, reasoned atomic.Bool
	resp, err := provider.Stream(sctx, messages, nil, func(chunk llm.StreamChunk) {
		if chunk.ReasoningDelta != "" {
			reasoned.Store(true)
		}
		if chunk.TextDelta == "" || cut.Load() {
			return
		}
		text.WriteString(chunk.TextDelta)
		if cutoffReached(text.String(), multiLine, indent) {
			cut.Store(true)
			cancel()
		}
	})
	if resp != nil && resp.Reasoning != "" {
		reasoned.Store(true)
	}
	if cut.Load() {
		return streamedAnswer{text: text.String(), resp: resp, reasoned: reasoned.Load()}, nil
	}
	if err != nil {
		return streamedAnswer{}, err
	}
	if text.Len() == 0 && resp != nil {
		return streamedAnswer{text: resp.Content, resp: resp, reasoned: reasoned.Load()}, nil
	}
	return streamedAnswer{text: text.String(), resp: resp, reasoned: reasoned.Load()}, nil
}

// chatPrompt renders the chat-mode user message: related files first, then a short header so the
// model knows the language, then the window around the caret with the caret marked.
func chatPrompt(req holeRequest) string {
	var b strings.Builder
	if len(req.files) > 0 {
		b.WriteString("Related files (for reference only):\n")
		for _, f := range req.files {
			b.WriteString("```")
			b.WriteString(f.Path)
			b.WriteString("\n")
			b.WriteString(strings.TrimRight(f.Content, "\n"))
			b.WriteString("\n```\n")
		}
		b.WriteString("\nFile being edited:\n")
	}
	if p := strings.TrimSpace(req.path); p != "" {
		b.WriteString("File: ")
		b.WriteString(p)
		b.WriteString("\n")
	}
	if l := strings.TrimSpace(req.language); l != "" {
		b.WriteString("Language: ")
		b.WriteString(l)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(req.prefix)
	b.WriteString(autocompleteCursor)
	b.WriteString(req.suffix)
	return b.String()
}

// autocompleteModelID resolves which model completes: autocomplete.model when set, else the agent
// model, else the first configured entry - the same ladder as enhanceProvider.
func autocompleteModelID(cfg *config.Config, ac *config.AutocompleteConfig) (string, error) {
	modelID := strings.TrimSpace(ac.Model)
	if modelID == "" {
		modelID = strings.TrimSpace(cfg.Agent.Model)
	}
	if modelID == "" && len(cfg.Models) > 0 {
		modelID = cfg.Models[0].Model
	}
	if modelID == "" {
		return "", fmt.Errorf("no model configured")
	}
	return modelID, nil
}

// autocompleteProvider builds the LLM for one suggestion.
//
// Several things differ from the agent transport on purpose. The model entry's max_tokens is
// capped to autocomplete.max_tokens (the way internal/agent's titleProvider caps the title pass),
// so sharing one models[] entry with the agent cannot buy an 8k-token suggestion. Retries are
// off: a retried suggestion lands after the user has typed past it. Sampling is greedy unless
// autocomplete.temperature says otherwise, thinking is pinned off for models where it is a
// serving default, and the stop sequences end generation where the suggestion ends.
func (s *Server) autocompleteProvider(cfg *config.Config, ac *config.AutocompleteConfig, stops []string) (llm.Provider, error) {
	modelID, err := autocompleteModelID(cfg, ac)
	if err != nil {
		return nil, err
	}
	rm, err := cfg.ResolveLLM(modelID)
	if err != nil {
		return nil, err
	}
	if limit := ac.MaxTokens; limit > 0 && (rm.MaxTokens <= 0 || rm.MaxTokens > limit) {
		rm.MaxTokens = limit
	}
	mk := s.agentProviderFactory
	if mk == nil {
		mk = llm.NewProvider
	}
	return mk(llm.ProviderInput{
		Type:          rm.ProviderType,
		Model:         rm.Model,
		APIKey:        rm.APIKey,
		BaseURL:       rm.BaseURL,
		ProxyURL:      rm.ProxyURL,
		AuthPath:      rm.AuthPath,
		MaxTokens:     rm.MaxTokens,
		Temperature:   ac.Temperature,
		Deterministic: ac.Temperature <= 0,
		NoThinking:    true,
		Stop:          stops,
		RetryDisabled: true,
		DisableStream: !rm.Stream,
		Timeout:       time.Duration(ac.TimeoutMS) * time.Millisecond,
	})
}

// relatedFiles excerpts the other files the IDE reports open (POST /foxxycode/ide/editor-state):
// their first lines hold the imports and signatures the model needs to complete a call into
// them. Only files inside the server's workspace are read - an editor may have anything open,
// and a suggestion request carries no permission prompt.
func (s *Server) relatedFiles(current string, limit int) []llm.FIMFile {
	if limit <= 0 {
		return nil
	}
	root, err := filepath.Abs(strings.TrimSpace(s.defaultCWD))
	if err != nil || strings.TrimSpace(s.defaultCWD) == "" {
		return nil
	}
	currentAbs, _ := filepath.Abs(current)

	var out []llm.FIMFile
	for _, p := range ideenv.Get().OpenFiles {
		if len(out) >= limit {
			break
		}
		abs, err := filepath.Abs(p)
		if err != nil || strings.EqualFold(abs, currentAbs) {
			continue
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		excerpt, ok := readExcerpt(abs)
		if !ok {
			continue
		}
		out = append(out, llm.FIMFile{Path: filepath.ToSlash(rel), Content: excerpt})
	}
	return out
}

// readExcerpt returns the head of a text file, decoded the way every file tool decodes workspace
// bytes (internal/textenc), capped by lines and bytes. Binary and empty files are skipped.
func readExcerpt(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, relatedFileReadBytes))
	if err != nil || len(data) == 0 {
		return "", false
	}
	text, _, err := textenc.Decode(data)
	if err != nil || strings.ContainsRune(text, 0) {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > relatedFileLines {
		lines = lines[:relatedFileLines]
	}
	excerpt := headBytes(strings.Join(lines, "\n"), relatedFileBytes)
	if strings.TrimSpace(excerpt) == "" {
		return "", false
	}
	return excerpt, true
}

// displayPath shows the model a workspace-relative path when the file is inside the workspace,
// matching how related files are named, and the bare file name otherwise.
func (s *Server) displayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if root := strings.TrimSpace(s.defaultCWD); root != "" {
		if rootAbs, err := filepath.Abs(root); err == nil {
			if abs, err := filepath.Abs(path); err == nil {
				if rel, err := filepath.Rel(rootAbs, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return filepath.ToSlash(rel)
				}
			}
		}
	}
	return filepath.Base(path)
}
