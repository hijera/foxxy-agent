# Project rules

FoxxyCode discovers project rules from the session working directory and injects them into the system prompt via **`{{.Rules}}`**, separate from skills (**`{{.Skills}}`**).

## Prompt order

From top to bottom in the rendered system message:

1. Tools
2. Skills
3. Plan context / todo list (mode-dependent)
4. **Rules** (project docs + active rules)
5. Session memory
6. Current UTC time

## Discovery

When `rules.auto_discover` is true (default), FoxxyCode scans:

| Source | Path |
|--------|------|
| FoxxyCode | `.foxxyrules` or `.foxyrules` (single file **or** directory), and `.foxxycode/rules/` |
| Cursor | `.cursor/rules/` |
| Claude | `.claude/rules/` |
| Codex | `.codex/rules/` (markdown only in v1) |
| Agents | nested `**/AGENTS.md` ([agents.md](https://agents.md/) convention; hidden dirs, `node_modules`, `vendor` skipped) |

Duplicate rule files (same basename) resolve with precedence: **foxxycode > cursor > claude > codex > agents**. Nested `AGENTS.md` files are keyed by full path, so they never collapse into each other.

Nested `AGENTS.md` files are always-loaded: no frontmatter, no globs, active immediately. The **root** `AGENTS.md` is not part of this set — it already enters the prompt unconditionally as a project docs preamble (below).

### `.foxxyrules` / `.foxyrules` (top-level project rules)

Cline-style top-level project rules. Each root is accepted as **either** a single file **or** a directory:

- **Single file** (`.foxxyrules` / `.foxyrules`) without frontmatter is **always-loaded** for the session. With frontmatter it honors `globs` / `alwaysApply` like any other rule.
- **Directory** (`.foxxyrules/*.md`) loads its markdown files like the `.foxxycode/rules/` catalog.
- `.foxyrules` (one `x`) is an exact alias of `.foxxyrules`. Both count as the **FoxxyCode** source and win basename ties over cursor/claude/codex/agents.

CLI: `foxxycode rules list [--cwd DIR]` prints the discovered catalog.

## Activation

| Frontmatter | Behavior |
|-------------|----------|
| `alwaysApply: true` + `globs` / `paths` | Body enters **`{{.Rules}}`** only after the first glob match on a turn (from `file://` context or tool paths). Then **sticks** for the rest of the session |
| `alwaysApply: true` without globs | Active immediately for the session |
| `alwaysApply: false` | **Never** auto-included, even if globs would match. Body only when **`@ruleName`** appears in the user message |
| No frontmatter | Treated as auto; active immediately |

Mention-only rules use **`@name`** (file stem). They are **not** slash commands and do not appear in the skills catalog.

## Project docs preamble

On every turn, if present in session CWD:

- **`AGENTS.md`** (subsection `### AGENTS.md`)
- **`DESIGN.md`** (subsection `### DESIGN.md`)

These are unconditional; they do not use `alwaysApply` or `@mention`.

## Generating rules

Use the bundled skill **`/generate-rules`**. It is always available (embedded in the binary) and guides the agent to write focused `.foxxycode/rules/*.mdc` files via filesystem tools.

There is no `foxxycode rules generate` CLI subcommand.

## Context breakdown (UI)

After each agent turn, FoxxyCode estimates tokens per category (`systemPrompt`, `toolDefinitions`, `rules`, `skills`, `mcp`, `conversation`) and exposes them on **`GET /foxxycode/sessions/{id}/stats`** as `contextBreakdown`. The composer context ring opens a breakdown popover on click.

## Configuration

```yaml
rules:
  auto_discover: true
  systems: []   # optional filter: foxxycode, cursor, claude, codex, agents
```

## References

- [Cursor Rules](https://cursor.com/docs/rules)
- [Claude `.claude/rules`](https://code.claude.com/docs/en/memory#organize-rules-with-clauderules)
- [Codex Rules](https://developers.openai.com/codex/rules)
- Implementation: `internal/rules/*`, wiring in `internal/session`, `internal/agent/system_prompt.go`
