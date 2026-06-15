//go:build gateway || gateway.telegram

package telegram

import (
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// inputRichMessage mirrors the Bot API 10.1 InputRichMessage object.
// Exactly one of Markdown or HTML must be set; the gateway always uses Markdown.
type inputRichMessage struct {
	Markdown            string `json:"markdown,omitempty"`
	HTML                string `json:"html,omitempty"`
	IsRTL               bool   `json:"is_rtl,omitempty"`
	SkipEntityDetection bool   `json:"skip_entity_detection,omitempty"`
}

// replyParameters is the subset of Bot API ReplyParameters used by the gateway.
type replyParameters struct {
	MessageID int `json:"message_id"`
}

// richParams builds the form parameters for the sendRichMessage method.
// replyTo, when non-zero, threads the response as a reply to the user's message.
func richParams(chatID int64, markdown string, replyTo int) tgbotapi.Params {
	p := tgbotapi.Params{}
	p["chat_id"] = strconv.FormatInt(chatID, 10)
	_ = p.AddInterface("rich_message", inputRichMessage{Markdown: markdown})
	if replyTo != 0 {
		_ = p.AddInterface("reply_parameters", replyParameters{MessageID: replyTo})
	}
	return p
}

// richDraftParams builds the form parameters for the sendRichMessageDraft method.
// draftID must be non-zero; reusing it across calls animates the streamed changes.
func richDraftParams(chatID, draftID int64, markdown string) tgbotapi.Params {
	p := tgbotapi.Params{}
	p["chat_id"] = strconv.FormatInt(chatID, 10)
	p["draft_id"] = strconv.FormatInt(draftID, 10)
	_ = p.AddInterface("rich_message", inputRichMessage{Markdown: markdown})
	return p
}

// sendRichMessage sends a persistent rich message and returns the API response.
// The Bot API server must support Bot API 10.1; callers should fall back to the
// legacy formatted send when this returns an error.
func sendRichMessage(bot *tgbotapi.BotAPI, chatID int64, markdown string, replyTo int) (*tgbotapi.APIResponse, error) {
	resp, err := bot.MakeRequest("sendRichMessage", richParams(chatID, markdown, replyTo))
	if err != nil {
		return resp, fmt.Errorf("sendRichMessage: %w", err)
	}
	return resp, nil
}

// sendRichMessageDraft streams an ephemeral partial rich message (30-second preview).
// Drafts are private-chat only and auto-expire, so no deletion is required; the turn
// is finalized by a subsequent sendRichMessage call.
func sendRichMessageDraft(bot *tgbotapi.BotAPI, chatID, draftID int64, markdown string) error {
	if _, err := bot.MakeRequest("sendRichMessageDraft", richDraftParams(chatID, draftID, markdown)); err != nil {
		return fmt.Errorf("sendRichMessageDraft: %w", err)
	}
	return nil
}
