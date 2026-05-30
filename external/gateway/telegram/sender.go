//go:build gateway || gateway.telegram

package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// draftInterval is how often sendMessageDraft is called while tokens stream in.
	draftInterval = 2 * time.Second
	// typingInterval is how often the "typing…" action is refreshed (Telegram shows it for 5s).
	typingInterval = 4 * time.Second
)

// Sender implements acp.UpdateSender and streams agent output back to a Telegram chat.
// While the agent runs it sends periodic sendMessageDraft previews so the user sees
// the response building up in real time. Flush() sends the final persistent message.
type Sender struct {
	bot     *tgbotapi.BotAPI
	chatID  int64
	replyTo int // original user message ID used for the first reply

	log *slog.Logger

	mu          sync.Mutex
	buf         strings.Builder
	lastTyping  time.Time
	lastDraft   time.Time
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
			text := s.buf.String()
			now := time.Now()
			wantDraft := now.Sub(s.lastDraft) >= draftInterval
			wantTyping := now.Sub(s.lastTyping) >= typingInterval
			if wantDraft {
				s.lastDraft = now
			}
			if wantTyping {
				s.lastTyping = now
			}
			s.mu.Unlock()

			if wantTyping {
				s.sendTyping()
			}
			if wantDraft {
				s.sendDraft(text)
			}
		}

	case acp.ToolCallUpdate:
		title := u.Title
		if title == "" {
			title = u.Kind
		}
		if title != "" {
			s.mu.Lock()
			s.buf.WriteString("\n⚙️ " + title + "…\n")
			now := time.Now()
			wantTyping := now.Sub(s.lastTyping) >= typingInterval
			if wantTyping {
				s.lastTyping = now
			}
			s.mu.Unlock()
			if wantTyping {
				s.sendTyping()
			}
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

// Flush sends all accumulated text as a final persistent message.
// Call after the agent turn completes.
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

// sendTyping refreshes the "typing…" indicator in the chat header.
func (s *Sender) sendTyping() {
	if _, err := s.bot.Request(tgbotapi.NewChatAction(s.chatID, tgbotapi.ChatTyping)); err != nil {
		s.log.Debug("telegram: typing action", "err", err)
	}
}

// sendDraft calls the Bot API sendMessageDraft method (available since Bot API 9.3)
// to show an animated preview of the response as it streams in.
// Errors are non-fatal: old clients or unsupported chat types will fall back
// to the final sendMessage from Flush().
func (s *Sender) sendDraft(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	params := tgbotapi.Params{
		"chat_id": fmt.Sprintf("%d", s.chatID),
		"text":    truncate(text, 4096),
	}
	s.mu.Lock()
	replyTo := s.replyTo
	s.mu.Unlock()
	if replyTo != 0 {
		// reply_parameters is a JSON object (introduced in Bot API 6.0).
		params["reply_parameters"] = fmt.Sprintf(`{"message_id":%d}`, replyTo)
	}
	if _, err := s.bot.MakeRequest("sendMessageDraft", params); err != nil {
		s.log.Debug("telegram: sendMessageDraft", "err", err)
	}
}

func (s *Sender) sendText(text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, chunk := range splitMessage(text, 4096) {
		msg := tgbotapi.NewMessage(s.chatID, chunk)
		s.mu.Lock()
		if s.replyTo != 0 {
			msg.ReplyToMessageID = s.replyTo
			s.replyTo = 0
		}
		s.mu.Unlock()
		if _, err := s.bot.Send(msg); err != nil {
			s.log.Warn("telegram send failed", "err", err)
			return err
		}
	}
	return nil
}

// truncate returns the first n bytes of s, cutting cleanly on a UTF-8 boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
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
