//go:build gateway || gateway.telegram

package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestRichParams_WireFormat(t *testing.T) {
	p := richParams(12345, "**hi**", 678)
	if p["chat_id"] != "12345" {
		t.Fatalf("chat_id: want 12345 got %q", p["chat_id"])
	}
	var rm map[string]any
	if err := json.Unmarshal([]byte(p["rich_message"]), &rm); err != nil {
		t.Fatalf("rich_message is not valid JSON: %v (%q)", err, p["rich_message"])
	}
	if rm["markdown"] != "**hi**" {
		t.Fatalf("rich_message.markdown: want **hi** got %v", rm["markdown"])
	}
	if _, ok := rm["html"]; ok {
		t.Fatalf("rich_message must not carry html when markdown is used: %q", p["rich_message"])
	}
	var rp map[string]any
	if err := json.Unmarshal([]byte(p["reply_parameters"]), &rp); err != nil {
		t.Fatalf("reply_parameters is not valid JSON: %v (%q)", err, p["reply_parameters"])
	}
	if rp["message_id"] != float64(678) {
		t.Fatalf("reply_parameters.message_id: want 678 got %v", rp["message_id"])
	}
}

func TestRichParams_NoReplyWhenZero(t *testing.T) {
	p := richParams(1, "x", 0)
	if _, ok := p["reply_parameters"]; ok {
		t.Fatalf("reply_parameters must be omitted when replyTo == 0")
	}
}

func TestRichDraftParams_WireFormat(t *testing.T) {
	p := richDraftParams(12345, 99, "wip")
	if p["chat_id"] != "12345" {
		t.Fatalf("chat_id: want 12345 got %q", p["chat_id"])
	}
	if p["draft_id"] != "99" {
		t.Fatalf("draft_id: want 99 got %q", p["draft_id"])
	}
	var rm map[string]any
	if err := json.Unmarshal([]byte(p["rich_message"]), &rm); err != nil {
		t.Fatalf("rich_message is not valid JSON: %v", err)
	}
	if rm["markdown"] != "wip" {
		t.Fatalf("rich_message.markdown: want wip got %v", rm["markdown"])
	}
}

// stubBot builds a BotAPI pointed at a local test server, bypassing the getMe
// network call that NewBotAPIWithClient would make.
func stubBot(t *testing.T, srvURL string) *tgbotapi.BotAPI {
	t.Helper()
	bot := &tgbotapi.BotAPI{Token: "TESTTOKEN", Client: &http.Client{}, Buffer: 100}
	bot.SetAPIEndpoint(srvURL + "/bot%s/%s")
	return bot
}

func TestSendRichMessage_PostsExpectedRequest(t *testing.T) {
	var gotPath string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":5,"type":"private"}}}`))
	}))
	defer srv.Close()

	resp, err := sendRichMessage(stubBot(t, srv.URL), 5, "# Title", 0)
	if err != nil {
		t.Fatalf("sendRichMessage: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok response")
	}
	if gotPath != "/botTESTTOKEN/sendRichMessage" {
		t.Fatalf("endpoint: want /botTESTTOKEN/sendRichMessage got %q", gotPath)
	}
	if gotForm.Get("chat_id") != "5" {
		t.Fatalf("chat_id: want 5 got %q", gotForm.Get("chat_id"))
	}
	var rm map[string]any
	if err := json.Unmarshal([]byte(gotForm.Get("rich_message")), &rm); err != nil {
		t.Fatalf("rich_message not JSON: %v", err)
	}
	if rm["markdown"] != "# Title" {
		t.Fatalf("markdown: want '# Title' got %v", rm["markdown"])
	}
}

func TestSendRichMessageDraft_PostsExpectedRequest(t *testing.T) {
	var gotPath string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	if err := sendRichMessageDraft(stubBot(t, srv.URL), 5, 42, "partial"); err != nil {
		t.Fatalf("sendRichMessageDraft: %v", err)
	}
	if gotPath != "/botTESTTOKEN/sendRichMessageDraft" {
		t.Fatalf("endpoint: want /botTESTTOKEN/sendRichMessageDraft got %q", gotPath)
	}
	if gotForm.Get("draft_id") != "42" {
		t.Fatalf("draft_id: want 42 got %q", gotForm.Get("draft_id"))
	}
}
