package agent

import (
	"context"
	_ "embed"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/prompts"
	"github.com/hijera/foxxycode-agent/internal/session"
)

//go:embed prompts/title.md
var titleSystemPrompt string

// titleMaxRunes clamps the final title, mirroring the KiloCode reference (~100 chars).
const titleMaxRunes = 100

// titleProviderForConfig returns the provider used for the title pass: a dedicated model when title.model is
// configured, otherwise the passed-in main provider. Any resolution error falls back to the main
// provider rather than failing the turn. Mirrors compactionProvider.
//
// Title generation runs asynchronously, while config_commit can replace a.cfg in the turn goroutine,
// so the background pass must not read a.cfg itself.
func (a *Agent) titleProviderForConfig(fallback llm.Provider, cfg *config.Config) llm.Provider {
	if cfg == nil {
		return fallback
	}
	ref := strings.TrimSpace(cfg.Title.Model)
	if ref == "" {
		return fallback
	}
	rm, err := cfg.ResolveLLM(ref)
	if err != nil || rm == nil {
		return fallback
	}
	if cap := cfg.Title.MaxTokens; cap > 0 && (rm.MaxTokens <= 0 || rm.MaxTokens > cap) {
		rm.MaxTokens = cap
	}
	mk := a.providerFactory
	if mk == nil {
		mk = llm.NewProvider
	}
	p, err := mk(a.llmProviderInputForConfig(cfg, rm))
	if err != nil || p == nil {
		return fallback
	}
	return p
}

// maybeGenerateTitle generates a short LLM session title once, after the first exchange, when the
// session has no user-pinned title and no auto-title yet. It persists the title and broadcasts a
// SessionTitleUpdate so all clients update live. All failures are non-fatal: the turn continues and
// the derived first-message title remains as a fallback. Runs off the hot path (goroutine caller).
func (a *Agent) maybeGenerateTitle(ctx context.Context, provider llm.Provider) {
	a.maybeGenerateTitleForConfig(ctx, provider, a.cfg)
}

// maybeGenerateTitleForConfig generates a title using the configuration that was current when
// the work was scheduled. It keeps the detached title pass independent from later config reloads.
func (a *Agent) maybeGenerateTitleForConfig(ctx context.Context, provider llm.Provider, cfg *config.Config) {
	if cfg == nil || !cfg.Title.TitleEnabled() {
		return
	}
	// Never override a user pin, and generate at most once per session.
	if strings.TrimSpace(a.state.GetTitlePinned()) != "" || strings.TrimSpace(a.state.GetTitleAuto()) != "" {
		return
	}

	firstUser := firstUserMessageContent(a.state.GetMessages())
	if firstUser == "" {
		return
	}

	p := a.titleProviderForConfig(provider, cfg)
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: prompts.WithIdentity(titleSystemPrompt)},
		{Role: llm.RoleUser, Content: "Generate a title for this conversation:\n" + firstUser},
	}
	resp, err := p.Complete(ctx, msgs, nil)
	if err != nil || resp == nil {
		if err != nil && a.log != nil {
			a.log.Warn("session title generation failed", "err", err)
		}
		return
	}

	title := cleanTitle(resp.Content)
	if title == "" {
		return
	}

	a.state.SetTitleAuto(title)
	_ = a.server.SendSessionUpdate(a.state.GetID(), acp.SessionTitleUpdate{
		SessionUpdate: acp.UpdateTypeSessionTitle,
		Title:         title,
	})
	if a.log != nil {
		a.log.Info("session title generated", "title", title)
	}
}

// firstUserMessageContent returns the first non-empty user message content with any injected
// <foxxycode_session_assets> block removed, or "" when none exists.
func firstUserMessageContent(history []llm.Message) string {
	for _, m := range history {
		if m.Role != llm.RoleUser {
			continue
		}
		text := strings.TrimSpace(session.StripInjectedContextBlocks(m.Content))
		if text != "" {
			return text
		}
	}
	return ""
}

// cleanTitle strips <think> reasoning, takes the first non-empty line, and clamps to titleMaxRunes.
func cleanTitle(raw string) string {
	s := raw
	for {
		start := strings.Index(strings.ToLower(s), "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(strings.ToLower(s[start:]), "</think>")
		if end < 0 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+end+len("</think>"):]
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "\"'")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rs := []rune(line)
		if len(rs) > titleMaxRunes {
			return strings.TrimSpace(string(rs[:titleMaxRunes-1])) + "…"
		}
		return line
	}
	return ""
}
