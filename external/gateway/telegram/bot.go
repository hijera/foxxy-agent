//go:build gateway || gateway.telegram

// Package telegram implements the Telegram bot adapter for the Coddy gateway.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/EvilFreelancer/coddy-agent/external/gateway/access"
	"github.com/EvilFreelancer/coddy-agent/external/gateway/sessionstore"
	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const adapterName = "tg"

// SessionRunner abstracts the session management and agent execution needed by the bot.
type SessionRunner interface {
	EnsureHTTPSession(ctx context.Context, sessionID string, defaultCWD string) (*session.State, error)
	HandleSessionPromptWithSender(ctx context.Context, params acp.SessionPromptParams, sender acp.UpdateSender, opts *session.PromptRunOpts) (*acp.SessionPromptResult, error)
	ForgetLiveSession(sessionID string)
}

// Bot is the Telegram gateway adapter.
type Bot struct {
	cfg     *config.TelegramGatewayConfig
	runner  SessionRunner
	cwd     string
	log     *slog.Logger
	store   *sessionstore.Store
	botName string // @username of the bot (set after connect)
}

// New creates a Bot. cwd is the default working directory for agent sessions.
func New(cfg *config.TelegramGatewayConfig, runner SessionRunner, cwd string, log *slog.Logger) *Bot {
	return &Bot{
		cfg:    cfg,
		runner: runner,
		cwd:    cwd,
		log:    log,
		store:  sessionstore.New(),
	}
}

// Name satisfies gateway.Adapter.
func (b *Bot) Name() string { return "telegram" }

// Start connects to Telegram and begins polling. Blocks until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) error {
	bot, err := tgbotapi.NewBotAPI(b.cfg.Token)
	if err != nil {
		return fmt.Errorf("telegram: connect: %w", err)
	}
	b.botName = bot.Self.UserName
	b.log.Info("telegram bot connected", "username", b.botName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			bot.StopReceivingUpdates()
			return nil
		case upd, ok := <-updates:
			if !ok {
				return fmt.Errorf("telegram: updates channel closed")
			}
			if upd.Message == nil {
				continue
			}
			go b.handleMessage(ctx, bot, upd.Message)
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	if msg.From == nil {
		return
	}

	userID := msg.From.ID
	chatID := msg.Chat.ID
	isGroup := msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() || msg.Chat.IsChannel()

	// --- Access control ---
	level := access.EffectiveAccess(chatID, b.cfg)
	if !access.CanAccess(userID, level, b.cfg) {
		b.log.Debug("telegram: access denied", "user", userID, "chat", chatID)
		return
	}

	// --- Isolation / admin-only mode ---
	isolation := access.EffectiveIsolation(chatID, b.cfg)
	if isGroup && isolation == config.IsolationAdmin && !b.cfg.IsAdmin(userID) {
		return
	}

	// --- Trigger check for group chats ---
	text := strings.TrimSpace(msg.Text)
	if isGroup && !b.shouldRespond(msg, text) {
		return
	}

	// --- /clear command ---
	if isCommand(msg, "clear") {
		key := sessionstore.SessionKey(adapterName, chatID, userID, isolation, isGroup)
		oldID := b.store.Get(key)
		newID := b.store.Reset(key)
		b.runner.ForgetLiveSession(oldID)
		reply(bot, chatID, msg.MessageID, "🔄 New session started.")
		b.log.Info("telegram: session cleared", "old", oldID, "new", newID, "user", userID)
		return
	}

	// --- Skip empty or command-only messages ---
	if text == "" || (msg.IsCommand() && msg.Command() != "start") {
		return
	}

	// Strip @mention prefix if present.
	text = stripMention(text, b.botName)
	if strings.TrimSpace(text) == "" {
		return
	}

	// --- Get or create session ---
	key := sessionstore.SessionKey(adapterName, chatID, userID, isolation, isGroup)
	sessionID := b.store.Get(key)

	ctx2, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	st, err := b.runner.EnsureHTTPSession(ctx2, sessionID, b.cwd)
	if err != nil {
		b.log.Warn("telegram: ensure session", "err", err)
		reply(bot, chatID, msg.MessageID, "❌ Failed to start session: "+err.Error())
		return
	}

	sender := newSender(bot, chatID, msg.MessageID, b.log)

	_, err = b.runner.HandleSessionPromptWithSender(ctx2, acp.SessionPromptParams{
		SessionID: st.GetID(),
		Prompt:    []acp.ContentBlock{{Type: "text", Text: text}},
	}, sender, nil)
	sender.Flush()

	if err != nil {
		b.log.Warn("telegram: agent error", "err", err, "session", sessionID)
		reply(bot, chatID, msg.MessageID, "❌ Agent error: "+err.Error())
	}
}

// shouldRespond checks whether the bot should process a group message.
// It responds to: direct @-mentions, replies to the bot, and /clear command.
func (b *Bot) shouldRespond(msg *tgbotapi.Message, text string) bool {
	if isCommand(msg, "clear") {
		return true
	}
	if strings.Contains(text, "@"+b.botName) {
		return true
	}
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.UserName == b.botName {
		return true
	}
	return false
}

func isCommand(msg *tgbotapi.Message, cmd string) bool {
	return msg.IsCommand() && strings.EqualFold(msg.Command(), cmd)
}

func stripMention(text, botName string) string {
	if botName == "" {
		return text
	}
	mention := "@" + botName
	s := strings.TrimPrefix(text, mention)
	s = strings.ReplaceAll(s, mention, "")
	return strings.TrimSpace(s)
}

func reply(bot *tgbotapi.BotAPI, chatID int64, replyTo int, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if replyTo != 0 {
		msg.ReplyToMessageID = replyTo
	}
	if _, err := bot.Send(msg); err != nil {
		slog.Warn("telegram: send reply failed", "err", err)
	}
}
