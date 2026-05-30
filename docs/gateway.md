# Messenger Gateway

The messenger gateway lets you drive a Coddy agent directly from a chat application such as Telegram. The agent runs the same ReAct loop, tools, and skills as in the HTTP UI or ACP mode — the gateway is only a transport layer.

## Contents

- [Overview](#overview)
- [Build tags](#build-tags)
- [Quick start (Telegram)](#quick-start-telegram)
- [Configuration reference](#configuration-reference)
  - [Access levels](#access-levels)
  - [Session isolation modes](#session-isolation-modes)
  - [Per-chat overrides](#per-chat-overrides)
  - [User groups](#user-groups)
- [Running the gateway](#running-the-gateway)
- [Bot interaction model](#bot-interaction-model)
  - [Private chats](#private-chats)
  - [Group chats](#group-chats)
  - [Commands](#commands)
- [Writing a new adapter](#writing-a-new-adapter)
  - [1. Implement the Adapter interface](#1-implement-the-adapter-interface)
  - [2. Register in Start()](#2-register-in-start)
  - [3. Implement acp.UpdateSender](#3-implement-acpupdatesender)
  - [4. Add a build tag](#4-add-a-build-tag)
  - [5. Wire into hub.Start()](#5-wire-into-hubstart)
- [Session lifecycle](#session-lifecycle)
- [Security notes](#security-notes)

---

## Overview

```
Telegram / future messengers
         │  polling / webhooks
         ▼
  external/gateway/          ← build tag: gateway | gateway.telegram
    Hub (goroutine per adapter, auto-restart)
         │
         ▼
  sessionstore               ← maps chat+user context → Coddy session ID
         │                     /clear command replaces the stored ID
         ▼
  session.Manager            ← shared with coddy acp / coddy http
    HandleSessionPromptWithSender(...)
         │
         ▼
  ReAct agent loop           ← identical to HTTP and ACP paths
  (tools, skills, MCP, LLM)
         │
         ▼
  Sender (per-message)       ← buffers agent output, sends back to chat
```

Multiple gateways (Telegram today, Discord/Slack tomorrow) run in the same process and share the same session store.

---

## Build tags

| Tag | Includes |
|-----|----------|
| `gateway.telegram` | Telegram adapter only |
| `gateway` | all adapters (currently Telegram; a superset for future integrations) |

Build with one tag:

```bash
# Telegram only
make build TAGS="gateway.telegram"

# All gateways
make build TAGS="gateway"

# Combined with HTTP, UI, scheduler, memory
make build TAGS="http ui scheduler memory gateway"
```

Without either tag the `coddy gateway` subcommand is present in the binary but returns a "not compiled" error when invoked — all other subcommands are unaffected.

---

## Quick start (Telegram)

**Step 1 — create a bot**

Talk to [@BotFather](https://t.me/BotFather) on Telegram:

```
/newbot
```

Save the token it returns. **Never commit the token to version control.** Store it in an environment variable:

```bash
export TELEGRAM_BOT_TOKEN="<your-token>"
```

**Step 2 — find your Telegram user ID**

Send any message to [@userinfobot](https://t.me/userinfobot). It will reply with your user ID. That ID becomes the `admins` list entry.

**Step 3 — add the gateway config**

In `~/.coddy/config.yaml` (or wherever your `config.yaml` lives), add:

```yaml
gateways:
  telegram:
    enabled: true
    token: "${TELEGRAM_BOT_TOKEN}"
    admins: [98874093]           # your Telegram user ID
    default_access: "all"
    default_isolation: "individual"
```

**Step 4 — build and run**

```bash
make build TAGS="gateway.telegram"
./build/coddy gateway --config ~/.coddy/config.yaml
```

Open Telegram, find your bot, send a message. The agent replies in the same chat.

---

## Configuration reference

All gateway config lives under the `gateways` key in `config.yaml`.

```yaml
gateways:
  telegram:
    enabled: false
    token: "${TELEGRAM_BOT_TOKEN}"

    # Telegram user IDs with elevated privileges.
    # Admins always pass every access check regardless of default_access.
    admins: []

    # Default access level for chats without a per-chat override.
    # Values: "all" | "admins" | "group:<name>"
    default_access: "all"

    # Default session isolation for group chats without a per-chat override.
    # Values: "individual" | "shared" | "admin"
    default_isolation: "individual"

    # Named user groups for group-level access control.
    user_groups:
      - name: "devs"
        user_ids: [111222333, 444555666]

    # Per-chat overrides (optional). chat_id is negative for groups/supergroups.
    chats:
      - chat_id: -1001234567890
        isolation: "individual"
        access: "all"
      - chat_id: -1009876543210
        isolation: "admin"
        access: "admins"
```

### Access levels

| Value | Who can interact |
|-------|-----------------|
| `all` | Anyone who can write to the chat |
| `admins` | Only user IDs listed in `admins` |
| `group:<name>` | Members of the named `user_groups` entry (admins always pass) |

Access is checked on every incoming message. Denied messages are silently dropped.

### Session isolation modes

Applies to **group and supergroup chats only**. Private chats are always per-user regardless of this setting.

| Mode | Session scope |
|------|--------------|
| `individual` | Each group member gets their own private session |
| `shared` | All members of the group share one session |
| `admin` | Only admin users can interact; all admins share one session |

Example: a shared DevOps bot for a team chat uses `shared`. A personal assistant added to a group uses `individual`.

### Per-chat overrides

Use `chats` to override `isolation` and `access` for specific chats:

```yaml
gateways:
  telegram:
    default_access: "all"
    default_isolation: "individual"
    chats:
      - chat_id: -1001111111111   # team group: shared session, all members
        isolation: "shared"
        access: "all"
      - chat_id: -1002222222222   # private project: admins only
        isolation: "admin"
        access: "admins"
```

### User groups

Define named groups of Telegram user IDs and reference them in `access`:

```yaml
gateways:
  telegram:
    user_groups:
      - name: "devs"
        user_ids: [111, 222, 333]
      - name: "qa"
        user_ids: [444, 555]
    chats:
      - chat_id: -1001234567890
        access: "group:devs"    # only devs (+ admins) can use the bot here
```

---

## Running the gateway

```bash
coddy gateway [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `$CODDY_HOME/config.yaml` | Path to config file |
| `--home` | `~/.coddy` | Agent state directory (`CODDY_HOME`) |
| `--cwd` | process cwd | Default session working directory |
| `--sessions-dir` | `$CODDY_HOME/sessions` | Where session bundles are stored |
| `--log-level` | from config | `debug\|info\|warn\|error` |

Typical production invocation:

```bash
coddy gateway \
  --config /etc/coddy/config.yaml \
  --home /var/lib/coddy \
  --sessions-dir /var/lib/coddy/sessions
```

The process blocks until `SIGINT` or `SIGTERM`. Each adapter runs in its own goroutine with automatic restart on error (5-second backoff). Send `Ctrl+C` for a clean shutdown.

**With Docker Compose** — add a second service to your `docker-compose.yml`:

```yaml
services:
  gateway:
    image: ghcr.io/coddy-project/coddy-agent   # build with gateway tag, see below
    command: ["coddy", "gateway", "--config", "/config/config.yaml"]
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - coddy_home:/var/lib/coddy
    environment:
      - TELEGRAM_BOT_TOKEN
    restart: unless-stopped
```

> The published Docker image does not include the `gateway` tag by default. Build a custom image with `BUILD_TAGS=http,ui,scheduler,memory,gateway` (see `Dockerfile`).

---

## Bot interaction model

### Private chats

Every user who starts a private conversation with the bot gets their own isolated session. No configuration needed.

### Group chats

In a group the bot **only responds** when explicitly addressed. It will react to:

1. A message that **@mentions** the bot (`@coddy_agent_bot hello`)
2. A **direct reply** to a previous bot message
3. The `/clear` command

When `isolation` is `admin`, the bot additionally ignores everyone who is not in the `admins` list.

### Commands

| Command | Available to | Effect |
|---------|-------------|--------|
| `/clear` | all permitted users | Starts a new session for the current user/chat context. The old session is removed from memory (persisted history remains on disk). |
| `/start` | all users | Telegram default; silently ignored beyond the access check. |

---

## Writing a new adapter

To add, for example, a Discord adapter alongside Telegram, follow this pattern.

### 1. Implement the Adapter interface

Create `external/gateway/discord/bot.go`:

```go
//go:build gateway || gateway.discord

package discord

import (
    "context"
    "fmt"

    "github.com/EvilFreelancer/coddy-agent/external/gateway"
    "github.com/EvilFreelancer/coddy-agent/external/gateway/access"
    "github.com/EvilFreelancer/coddy-agent/external/gateway/sessionstore"
    "github.com/EvilFreelancer/coddy-agent/internal/config"
)

type Bot struct {
    cfg   *config.DiscordGatewayConfig  // add to config.GatewayConfig
    store *sessionstore.Store
    // ... discord client, session runner, logger
}

func New(cfg *config.DiscordGatewayConfig, runner SessionRunner, cwd string, log *slog.Logger) *Bot {
    return &Bot{cfg: cfg, store: sessionstore.New()}
}

// Name satisfies gateway.Adapter.
func (b *Bot) Name() string { return "discord" }

// Start connects and polls. Must block until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) error {
    // connect discord client
    // poll or use websocket events
    // for each message: call b.handleMessage(ctx, msg)
    return nil
}
```

The `SessionRunner` interface (`external/gateway/telegram/bot.go`) is what you need from the session manager:

```go
type SessionRunner interface {
    EnsureHTTPSession(ctx context.Context, sessionID string, defaultCWD string) (*session.State, error)
    HandleSessionPromptWithSender(ctx context.Context, params acp.SessionPromptParams, sender acp.UpdateSender, opts *session.PromptRunOpts) (*acp.SessionPromptResult, error)
    ForgetLiveSession(sessionID string)
}
```

`session.Manager` already satisfies this interface — pass it directly.

### 2. Register in Start()

In `external/gateway/start.go`, add a block for the new adapter next to the Telegram block:

```go
//go:build gateway || gateway.discord

if cfg.Gateways.Discord.Enabled {
    bot := discord.New(&cfg.Gateways.Discord, mgr, defaultCWD, log)
    adapters = append(adapters, bot)
}
```

Because the Telegram file uses `//go:build gateway || gateway.telegram` and the Discord file uses `//go:build gateway || gateway.discord`, adding the Discord code to `start.go` requires updating the build constraint on that file to include `|| gateway.discord` as well. The cleanest approach is to split `start.go` per-adapter and give each its own constraint file, then have a `start_base.go` (tagged `gateway || gateway.telegram || gateway.discord`) that defines the `Start` function skeleton.

For a simpler one-adapter project, a single `start.go` with `//go:build gateway || gateway.telegram` is sufficient.

### 3. Implement acp.UpdateSender

Each message dispatch needs a `Sender` that implements three methods:

```go
type UpdateSender interface {
    SendSessionUpdate(sessionID string, update interface{}) error
    RequestPermission(ctx context.Context, params acp.PermissionRequestParams) (*acp.PermissionResult, error)
    RequestQuestion(ctx context.Context, params acp.QuestionRequestParams) (*acp.QuestionResult, error)
}
```

- `SendSessionUpdate` receives streaming events: `acp.MessageChunkUpdate` carries a text delta in `update.Content.Text`; `acp.ToolCallUpdate` is a tool start notification. Buffer text chunks and send them as a single message in `Flush()` after the agent turn.
- `RequestPermission` should auto-approve in a gateway context (the admin configured the bot deliberately). Return `&acp.PermissionResult{Outcome: "allow", OptionID: "allow"}`.
- `RequestQuestion` can send the question text to the chat and return an empty answer, or implement a proper reply-based flow.

See `external/gateway/telegram/sender.go` for a working reference.

### 4. Add a build tag

Follow the existing pattern:

- `external/gateway/discord/*.go` → `//go:build gateway || gateway.discord`
- `external/gateway/discord/*_test.go` → same constraint
- Stub (if needed) → `//go:build !(gateway || gateway.discord)`

Update the `start.go` / `start_stub.go` constraint to include the new tag.

### 5. Wire into hub.Start()

`hub.Start()` accepts any `[]gateway.Adapter`. No changes to Hub itself are needed — just `append` your adapter before calling `hub.Start(ctx)`.

---

## Session lifecycle

```
User message arrives
        │
        ▼
sessionstore.SessionKey(gateway, chatID, userID, isolationMode, isGroup)
        │  returns a string key like "tg:chat:-100:user:42"
        ▼
store.Get(key)
        │  returns existing session ID, or mints a new one on first call
        ▼
manager.EnsureHTTPSession(ctx, sessionID, cwd)
        │  loads from disk if persisted, creates fresh otherwise
        ▼
manager.HandleSessionPromptWithSender(ctx, params, sender, nil)
        │  runs the ReAct loop; sends stream events to Sender
        ▼
sender.Flush()
        │  sends buffered text to the chat
        ▼
Session bundle written to disk ($CODDY_HOME/sessions/<id>/)
```

**`/clear` flow:**

```
store.Reset(key)   → replaces stored session ID with a new random one
manager.ForgetLiveSession(oldID)   → drops the in-memory session (disk persists)
Next message → EnsureHTTPSession creates a fresh session for the new ID
```

The old session files remain on disk under the old ID. Use `coddy sessions list` to inspect them.

---

## Security notes

- **Token exposure** — never commit the bot token to version control. Use `"${TELEGRAM_BOT_TOKEN}"` in YAML and export the variable before starting.
- **Permissions** — the gateway auto-approves all tool permission requests so the agent can work unattended. Restrict `tools.command_allowlist` in `config.yaml` if you want to limit which shell commands the agent can run.
- **Access control** — set `default_access: "admins"` for bots that should only respond to a specific set of users. Open bots (`default_access: "all"`) will respond to any Telegram user who can write to the chat.
- **Network** — the gateway uses Telegram long-polling (not webhooks). No inbound port needs to be open.
