---
description: Main internal packages at a glance
paths:
  - "internal/**/*.go"
---

# Core modules (sketch)

- **`internal/acp`** - ACP RPC server, session lifecycle from editors.
- **`internal/agent`** - tool loop and LLM turns.
- **`internal/session`** - session manager and mode (`agent` / `plan` / `docs` / `ask`).
- **`internal/config`** - YAML and flags.
- **`internal/mcp`** - MCP clients and transports, the merged server list, and the **workspace trust gate**. A project-local **`<cwd>/.foxxycode/mcp.json`** arrives with the checkout, so **`TrustGate`** (**`gate.go`**) is the single decision point between that list and a spawn or an outbound connection; it re-checks immediately before the transport opens. Approvals live in **`<home>/mcp-trust.json`** keyed by canonical workspace and bound to a digest of the command-bearing declaration (**`trust.go`**). **Never reach `Connect` / `Probe` around the gate for a configuration-derived server.** See **`docs/mcp-integration.md`**.
- **`internal/tools`** - filesystem, shell, todo, MCP merge, etc. Shell also owns the background task family (**`run_command`** **`background: true`** plus **`background_list`** / **`background_output`** / **`background_wait`** / **`background_stop`** / **`background_reap`**), backed by **`internal/bgtask`**. See **`docs/background-tasks.md`**.
- **`internal/bgtask`** - process-wide, session-scoped pool for work that outlives a tool call. A task is whatever a **`Runner`** starts; **`CommandRunner`** is the only implementation today and **`KindAgent`** is reserved for a future nested-agent runner, so that lands as a second **`Runner`** rather than a second scheduling mechanism. **`Pool.Adopt`** takes over a **foreground** command that outlived its timeout instead of killing it, sharing the single scheduling path with **`Start`**.
- **`internal/skills`** - skill loading, enable/disable, and neuraldeep.ru registry client. Default dirs: `~/.agents/skills` (global, shared with `npx skills`/`npx skillsbd`), `~/.foxxycode/skills` (foxxycode-specific), `${CWD}/.foxxycode/skills` (project-local). No `install_dir` — installation is handled externally by `npx skills` / `npx skillsbd` or the HTTP UI (Settings → Skills). See `docs/skills.md`.

Prefer extending these over growing **`cmd/`** or duplicating logic in **`external/`**.

## References

@architecture.md
