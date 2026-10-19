package autocomplete

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type runner struct {
	autocomplete         *state
	log                  *slog.Logger
	agentProviderFactory func(llm.ProviderInput) (llm.Provider, error)
}

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

// autocompleteRateLimitPause is the Retry-After sent to clients when the provider rate-limits a
// suggestion but names no pause of its own.
const RateLimitPause = 10 * time.Second

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
func (s *runner) completeHole(ctx context.Context, cfg *config.Config, ac *config.AutocompleteConfig, modelID string, req holeRequest) (string, string, *llm.Response, error) {
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
func (s *runner) autocompleteProvider(cfg *config.Config, ac *config.AutocompleteConfig, stops []string) (llm.Provider, error) {
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
		NoCLILogin:    rm.NoCLILogin,
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
