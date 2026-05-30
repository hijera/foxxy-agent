//go:build gateway || gateway.telegram

package telegram

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Sender implements acp.UpdateSender and streams agent output back to a Telegram chat.
// Text chunks are buffered and sent as a single message via Flush.
type Sender struct {
	bot     *tgbotapi.BotAPI
	chatID  int64
	replyTo int // original user message ID for the first reply

	log *slog.Logger

	mu  sync.Mutex
	buf strings.Builder
}

func newSender(bot *tgbotapi.BotAPI, chatID int64, replyTo int, log *slog.Logger) *Sender {
	return &Sender{bot: bot, chatID: chatID, replyTo: replyTo, log: log}
}

// SendSessionUpdate handles streaming events from the agent.
func (s *Sender) SendSessionUpdate(_ string, update interface{}) error {
	switch u := update.(type) {
	case acp.MessageChunkUpdate:
		if u.Content.Type == acp.ContentTypeText {
			s.mu.Lock()
			s.buf.WriteString(u.Content.Text)
			s.mu.Unlock()
		}
	case acp.ToolCallUpdate:
		// Show brief tool activity indicator.
		title := u.Title
		if title == "" {
			title = u.Kind
		}
		if title != "" {
			s.mu.Lock()
			s.buf.WriteString("\n⚙️ " + title + "…\n")
			s.mu.Unlock()
		}
	}
	return nil
}

// RequestPermission auto-approves in gateway context (no interactive UI).
func (s *Sender) RequestPermission(_ context.Context, _ acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	return &acp.PermissionResult{Outcome: "allow", OptionID: "allow"}, nil
}

// RequestQuestion sends the question text to Telegram and returns an empty answer.
func (s *Sender) RequestQuestion(_ context.Context, params acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	if len(params.Questions) > 0 {
		_ = s.sendText(params.Questions[0].Question)
	}
	return &acp.QuestionResult{}, nil
}

// Flush sends all accumulated text to Telegram. Call after the agent turn completes.
func (s *Sender) Flush() {
	s.mu.Lock()
	text := s.buf.String()
	s.buf.Reset()
	s.mu.Unlock()

	if strings.TrimSpace(text) == "" {
		return
	}
	_ = s.sendText(text)
}

func (s *Sender) sendText(text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, chunk := range splitMessage(text, 4096) {
		msg := tgbotapi.NewMessage(s.chatID, chunk)
		if s.replyTo != 0 {
			msg.ReplyToMessageID = s.replyTo
			s.replyTo = 0
		}
		if _, err := s.bot.Send(msg); err != nil {
			s.log.Warn("telegram send failed", "err", err)
			return err
		}
	}
	return nil
}

func splitMessage(text string, limit int) []string {
	if len(text) <= limit {
		return []string{text}
	}
	var parts []string
	for len(text) > limit {
		cut := limit
		for i := limit - 1; i > limit/2; i-- {
			if text[i] == '\n' {
				cut = i + 1
				break
			}
		}
		parts = append(parts, text[:cut])
		text = text[cut:]
	}
	if text != "" {
		parts = append(parts, text)
	}
	return parts
}
