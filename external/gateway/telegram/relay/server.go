//go:build gateway || gateway.telegram

// Package relay owns the single Bot API connection shared by editor backends.
package relay

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hijera/foxxycode-agent/external/gateway/access"
	"github.com/hijera/foxxycode-agent/external/gateway/proxyutil"
	"github.com/hijera/foxxycode-agent/internal/config"
)

const leaseTime = 75 * time.Second

// Registration describes a live project. Tokens stay in memory in the relay.
type Registration struct {
	ID      string
	Project string
	Label   string
	Editor  string
	APIBase string
	Config  config.TelegramGatewayConfig
}

type client struct {
	Registration
	seen    time.Time
	pending []tgbotapi.Update
	wake    chan struct{}
}

type upstream struct {
	token, base, proxy string
	username           string
	http               *http.Client
	cancel             context.CancelFunc
	started            bool
}

// Server multiplexes Bot API calls and routes updates to one project at a time.
type Server struct {
	ctx           context.Context
	secret        string
	mu            sync.Mutex
	clients       map[string]*client
	bots          map[string]*upstream
	selected      map[string]string
	owners        map[string]string
	menus         map[string][]string
	selectionPath string
	lastClient    time.Time
}

// NewServer creates an authenticated loopback service. Cancellation stops polls.
func NewServer(ctx context.Context, secret string) *Server {
	return &Server{ctx: ctx, secret: secret, clients: make(map[string]*client), bots: make(map[string]*upstream), selected: make(map[string]string), owners: make(map[string]string), menus: make(map[string][]string), lastClient: time.Now()}
}

// Register renews a client's lease without replacing its unacknowledged updates.
func (s *Server) Register(r Registration) error {
	if r.ID == "" || r.Project == "" || (r.Editor != "intellij" && r.Editor != "vscode") {
		return errors.New("invalid editor registration")
	}
	r.Config.Token = r.Config.EffectiveToken()
	if r.Config.Token == "" || strings.ContainsAny(r.Config.Token, "/?#") {
		return errors.New("invalid bot token")
	}
	r.APIBase = strings.TrimRight(strings.TrimSpace(r.APIBase), "/")
	if r.APIBase == "" {
		r.APIBase = "https://api.telegram.org"
	}
	u, err := url.Parse(r.APIBase)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid Bot API origin")
	}
	hc, err := proxyutil.BuildHTTPClient(r.Config.Proxy)
	if err != nil {
		return errors.New("invalid Telegram proxy")
	}
	// Bound requests and never include a URL containing the bot token in errors.
	copyClient := *hc
	copyClient.Timeout = 40 * time.Second
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	s.mu.Lock()
	defer s.mu.Unlock()
	if bot := s.bots[r.Config.Token]; bot != nil {
		if bot.base != r.APIBase || bot.proxy != r.Config.Proxy {
			return errors.New("projects sharing a bot must use the same Bot API origin and proxy")
		}
	} else {
		s.bots[r.Config.Token] = &upstream{token: r.Config.Token, base: r.APIBase, proxy: r.Config.Proxy, http: &copyClient}
	}
	if c := s.clients[r.ID]; c != nil {
		if c.Config.Token != r.Config.Token || c.Project != r.Project {
			return errors.New("client identity changed")
		}
		c.Registration = r
		c.seen = time.Now()
	} else {
		s.clients[r.ID] = &client{Registration: r, seen: time.Now(), wake: make(chan struct{}, 1)}
	}
	s.lastClient = time.Now()
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.secret)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/clients/") && r.Method == http.MethodDelete {
		s.mu.Lock()
		delete(s.clients, strings.TrimPrefix(r.URL.Path, "/clients/"))
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.Path == "/register" && r.Method == http.MethodPost {
		var reg Registration
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&reg); err != nil {
			http.Error(w, "invalid registration", http.StatusBadRequest)
			return
		}
		if err := s.Register(reg); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "api" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	c := s.clients[parts[1]]
	if c == nil {
		s.mu.Unlock()
		http.Error(w, "client expired", http.StatusGone)
		return
	}
	c.seen = time.Now()
	s.lastClient = c.seen
	bot := s.bots[c.Config.Token]
	registration := c.Registration
	s.mu.Unlock()
	if parts[2] == "getUpdates" {
		s.updates(w, r, c, bot)
		return
	}
	// An editor must never start a second upstream consumer or replace its subscription.
	if parts[2] == "setWebhook" || parts[2] == "deleteWebhook" {
		http.Error(w, "relay owns update delivery", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	if parts[2] == "setMyCommands" {
		body = withProjectCommand(body)
	}
	status, data, err := bot.call(r.Context(), parts[2], r.Header.Get("Content-Type"), body)
	if err != nil {
		http.Error(w, "Telegram request failed", http.StatusBadGateway)
		return
	}
	if parts[2] == "getMe" {
		var result struct {
			Result tgbotapi.User `json:"result"`
		}
		if json.Unmarshal(data, &result) == nil {
			s.mu.Lock()
			bot.username = result.Result.UserName
			s.mu.Unlock()
		}
	}
	s.rememberOwner(registration, data)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func withProjectCommand(body []byte) []byte {
	v, err := url.ParseQuery(string(body))
	if err != nil {
		return body
	}
	var cmds []tgbotapi.BotCommand
	if json.Unmarshal([]byte(v.Get("commands")), &cmds) != nil {
		return body
	}
	cmds = append(cmds, tgbotapi.BotCommand{Command: "project", Description: "Choose an IntelliJ or VS Code project"})
	data, _ := json.Marshal(cmds)
	v.Set("commands", string(data))
	return []byte(v.Encode())
}

func (b *upstream) call(ctx context.Context, method, contentType string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+"/bot"+b.token+"/"+method, strings.NewReader(string(body)))
	if err != nil {
		return 0, nil, errors.New("invalid Telegram request")
	}
	req.Header.Set("Content-Type", contentType)
	res, err := b.http.Do(req)
	if err != nil {
		return 0, nil, errors.New("telegram unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	return res.StatusCode, data, err
}

func (s *Server) updates(w http.ResponseWriter, r *http.Request, c *client, bot *upstream) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if r.ParseForm() != nil {
		http.Error(w, "invalid poll", http.StatusBadRequest)
		return
	}
	offset, _ := strconv.Atoi(r.Form.Get("offset"))
	s.mu.Lock()
	if !bot.started {
		ctx, cancel := context.WithCancel(s.ctx)
		bot.cancel = cancel
		bot.started = true
		go s.poll(ctx, bot)
	}
	s.mu.Unlock()
	timer := time.NewTimer(25 * time.Second)
	defer timer.Stop()
	for {
		s.mu.Lock()
		for len(c.pending) > 0 && c.pending[0].UpdateID < offset {
			c.pending = c.pending[1:]
		}
		pending := append([]tgbotapi.Update{}, c.pending...)
		s.mu.Unlock()
		if len(pending) > 0 {
			writeUpdates(w, pending)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case <-timer.C:
			writeUpdates(w, []tgbotapi.Update{})
			return
		case <-c.wake:
		}
	}
}

func writeUpdates(w http.ResponseWriter, updates []tgbotapi.Update) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": updates})
}

func (s *Server) poll(ctx context.Context, bot *upstream) {
	offset := 0
	for ctx.Err() == nil {
		form := url.Values{"offset": {strconv.Itoa(offset)}, "timeout": {"30"}, "allowed_updates": {`["message","callback_query"]`}}
		status, data, err := bot.call(ctx, "getUpdates", "application/x-www-form-urlencoded", []byte(form.Encode()))
		var result struct {
			OK         bool              `json:"ok"`
			Result     []tgbotapi.Update `json:"result"`
			Parameters struct {
				RetryAfter int `json:"retry_after"`
			} `json:"parameters"`
		}
		if err != nil || status != http.StatusOK || json.Unmarshal(data, &result) != nil || !result.OK {
			delay := 3 * time.Second
			if result.Parameters.RetryAfter > 0 && result.Parameters.RetryAfter < 300 {
				delay = time.Duration(result.Parameters.RetryAfter) * time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}
		for _, u := range result.Result {
			s.route(bot.token, u)
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}
}

func (s *Server) rememberOwner(c Registration, data []byte) {
	var res struct {
		OK     bool             `json:"ok"`
		Result tgbotapi.Message `json:"result"`
	}
	if json.Unmarshal(data, &res) != nil || !res.OK || res.Result.Chat == nil || res.Result.MessageID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Only messages carrying buttons need routing. Bound the cache even for long IDE runs.
	if res.Result.ReplyMarkup == nil {
		return
	}
	if len(s.owners) >= 4096 {
		s.owners = make(map[string]string)
	}
	s.owners[ownerKey(c.Config.Token, res.Result.Chat.ID, res.Result.MessageID)] = c.Project
}

func ownerKey(token string, chat int64, message int) string {
	return fmt.Sprintf("%s:%d:%d", token, chat, message)
}

func selectionKey(token string, chat, user int64, thread int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d", token, chat, user, thread)))
	return hex.EncodeToString(sum[:])
}

func (s *Server) loadSelections(path string) {
	s.selectionPath = path
	data, err := os.ReadFile(path) // #nosec G304 -- relay-owned user cache file
	if err == nil {
		var selected map[string]string
		if json.Unmarshal(data, &selected) == nil && selected != nil {
			s.selected = selected
		}
	}
}

// saveSelectionsLocked stores only hashed chat identities and opaque project IDs.
func (s *Server) saveSelectionsLocked() error {
	if s.selectionPath == "" {
		return nil
	}
	data, err := json.Marshal(s.selected)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.selectionPath), "selection-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.selectionPath)
}

// Sweep removes disconnected projects and stops bot polls once their last lease expires.
// It returns true when the whole relay has been idle long enough to exit.
func (s *Server) Sweep(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.clients {
		if now.Sub(c.seen) > leaseTime {
			delete(s.clients, id)
		}
	}
	for token, bot := range s.bots {
		active := false
		for _, c := range s.clients {
			if c.Config.Token == token {
				active = true
				break
			}
		}
		if !active {
			if bot.cancel != nil {
				bot.cancel()
			}
			delete(s.bots, token)
		}
	}
	return len(s.clients) == 0 && now.Sub(s.lastClient) > leaseTime
}

func (s *Server) route(token string, u tgbotapi.Update) {
	m := u.Message
	user := int64(0)
	if m != nil && m.From != nil {
		user = m.From.ID
	}
	if cb := u.CallbackQuery; cb != nil {
		m = cb.Message
		if cb.From != nil {
			user = cb.From.ID
		}
	}
	if m == nil || m.Chat == nil || user == 0 {
		return
	}
	s.mu.Lock()
	if u.CallbackQuery == nil && m.Chat.Type != "private" {
		bot := s.bots[token]
		if bot == nil || !addressed(m, bot.username) {
			s.mu.Unlock()
			return
		}
	}
	var choices []*client
	projects := make(map[string]*client)
	for _, c := range s.clients {
		if c.Config.Token != token || time.Since(c.seen) > leaseTime || !access.CanAccess(user, access.EffectiveAccess(m.Chat.ID, &c.Config), &c.Config) {
			continue
		}
		if access.EffectiveIsolation(m.Chat.ID, &c.Config) == config.IsolationAdmin && !c.Config.IsAdmin(user) {
			continue
		}
		if previous := projects[c.Project]; previous == nil || c.ID < previous.ID {
			projects[c.Project] = c
		}
	}
	for _, c := range projects {
		choices = append(choices, c)
	}
	sort.Slice(choices, func(i, j int) bool {
		if choices[i].Label != choices[j].Label {
			return choices[i].Label < choices[j].Label
		}
		return choices[i].Project < choices[j].Project
	})
	key := selectionKey(token, m.Chat.ID, user, 0)
	project := s.selected[key]
	if cb := u.CallbackQuery; cb != nil && strings.HasPrefix(cb.Data, "relay:project:") {
		project = strings.TrimPrefix(cb.Data, "relay:project:")
		var chosen *client
		for _, c := range choices {
			if c.Project == project {
				chosen = c
				break
			}
		}
		bot := s.bots[token]
		text := "Project is disconnected or unavailable. Open /project again."
		if chosen != nil {
			s.selected[key] = project
			text = "Selected project: " + chosen.Label
			if s.saveSelectionsLocked() != nil {
				text += " (choice saved for this run only)"
			}
		}
		s.mu.Unlock()
		s.answerCallback(bot, cb.ID, "Project selection updated")
		if len(choices) > 0 {
			s.notice(bot, m, text)
		}
		return
	} else if cb := u.CallbackQuery; cb != nil {
		project = s.owners[ownerKey(token, m.Chat.ID, m.MessageID)]
		if project == "" {
			bot := s.bots[token]
			s.mu.Unlock()
			s.answerCallback(bot, cb.ID, "This menu expired. Open it again in its project.")
			return
		}
	} else if strings.EqualFold(m.Command(), "project") {
		n, err := strconv.Atoi(strings.TrimSpace(m.CommandArguments()))
		wanted := strings.TrimSpace(m.CommandArguments())
		if err == nil && n > 0 && n <= len(s.menus[key]) {
			wanted = s.menus[key][n-1]
		}
		var chosen *client
		for _, c := range choices {
			if c.Project == wanted {
				chosen = c
				break
			}
		}
		var text string
		if chosen != nil {
			s.selected[key] = chosen.Project
			text = "Selected project: " + chosen.Label
			if s.saveSelectionsLocked() != nil {
				text += " (choice saved for this run only)"
			}
		} else {
			text = "Choose a project with /project <number>:"
			s.menus[key] = nil
			for i, c := range choices {
				text += fmt.Sprintf("\n%d. [%s] %s", i+1, c.Editor, c.Label)
			}
			for _, c := range choices {
				s.menus[key] = append(s.menus[key], c.Project)
			}
		}
		bot := s.bots[token]
		menu := make([]Registration, 0, len(choices))
		for _, c := range choices {
			menu = append(menu, c.Registration)
		}
		s.mu.Unlock()
		if len(choices) > 0 {
			if chosen != nil {
				s.notice(bot, m, text)
			} else {
				s.projectMenu(bot, m, text, menu)
			}
		}
		return
	}
	if project == "" && len(choices) == 1 {
		project = choices[0].Project
		s.selected[key] = project
		_ = s.saveSelectionsLocked()
	}
	for _, c := range choices {
		if c.Project == project {
			full := len(c.pending) >= 256
			if !full {
				c.pending = append(c.pending, u)
				select {
				case c.wake <- struct{}{}:
				default:
				}
			}
			bot := s.bots[token]
			s.mu.Unlock()
			if full {
				s.notice(bot, m, "The project's message queue is full. Please retry after its current work finishes.")
			}
			return
		}
	}
	bot := s.bots[token]
	s.mu.Unlock()
	if len(choices) > 0 && u.CallbackQuery == nil {
		s.notice(bot, m, "Choose a connected project with /project before sending a message.")
	}
	if cb := u.CallbackQuery; cb != nil {
		s.answerCallback(bot, cb.ID, "The project for this menu is unavailable. Open /project.")
	}
}

func (s *Server) answerCallback(bot *upstream, id, text string) {
	if bot == nil {
		return
	}
	v := url.Values{"callback_query_id": {id}, "text": {text}}
	_, _, _ = bot.call(s.ctx, "answerCallbackQuery", "application/x-www-form-urlencoded", []byte(v.Encode()))
}

func (s *Server) projectMenu(bot *upstream, m *tgbotapi.Message, text string, choices []Registration) {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, c := range choices {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData("["+c.Editor+"] "+c.Label, "relay:project:"+c.Project)})
	}
	markup, _ := json.Marshal(tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows})
	v := url.Values{"chat_id": {strconv.FormatInt(m.Chat.ID, 10)}, "text": {text}, "reply_markup": {string(markup)}}
	_, _, _ = bot.call(s.ctx, "sendMessage", "application/x-www-form-urlencoded", []byte(v.Encode()))
}

func addressed(m *tgbotapi.Message, username string) bool {
	if m.IsCommand() {
		command := strings.Fields(m.Text)[0]
		_, target, qualified := strings.Cut(command, "@")
		if qualified && !strings.EqualFold(target, username) {
			return false
		}
		return true
	}
	if username == "" {
		return false
	}
	if strings.Contains(m.Text, "@"+username) {
		return true
	}
	return m.ReplyToMessage != nil && m.ReplyToMessage.From != nil && m.ReplyToMessage.From.UserName == username
}

func (s *Server) notice(bot *upstream, m *tgbotapi.Message, text string) {
	if bot == nil {
		return
	}
	v := url.Values{"chat_id": {strconv.FormatInt(m.Chat.ID, 10)}, "text": {text}}
	_, _, _ = bot.call(s.ctx, "sendMessage", "application/x-www-form-urlencoded", []byte(v.Encode()))
}
