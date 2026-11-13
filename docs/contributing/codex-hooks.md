# Codex hooks for project rules

This page is about **contributors running the Codex CLI against this repository**. It is not about FoxxyCode's own rules discovery, which is a product feature documented in [Rules](../features/rules.md) - that one describes how the `foxxycode` binary injects `{{.Rules}}` into its system prompt.

## Why a hook is needed

Codex resolves its instruction chain **once per session**, walking from the repository root down to the directory it was launched in, and concatenating at most one `AGENTS.md` per directory. Two consequences matter here:

- a session started at the repository root never loads a nested `AGENTS.md`, no matter which files it goes on to edit;
- there is no glob-based attachment. Codex has no equivalent of the `globs` field in `.cursor/rules/*.mdc`, so scope can only be expressed by where a file sits in the tree.

This repository keeps a native Cursor representation under `.cursor/rules/*.mdc`, and the Codex adapter reads those files directly. A previous manually maintained index only asked the model to open another file; that was advisory and was skipped.

`.codex/hooks/attach_rules.py` closes the gap by injecting rule bodies into the session directly, so compliance no longer depends on the model choosing to follow an index.

## What fires when

| Event | Matcher | What is injected |
|---|---|---|
| `SessionStart` | `startup\|resume\|clear\|compact\|fork` | every rule whose frontmatter has `alwaysApply: true` |
| `PreToolUse` | `^(apply_patch\|Edit\|Write)$` | rules whose `globs` cover a file in the pending patch |

Configuration lives in `.codex/hooks.json`. Both handlers run the same script; it branches on `hook_event_name` from the hook payload. The default command uses `python3`; `commandWindows` uses `python`, because Windows may resolve `python3` to the Microsoft Store stub instead of the installed interpreter. Both commands require Python 3 on `PATH`. The `commandWindows` launcher walks up from the working directory to find `attach_rules.py` and exits 0 without attaching anything when it finds none, so a session outside the repository is not blocked by the hook.

The Windows launcher uses `python -X utf8` and finds `.codex/hooks/attach_rules.py`
by walking from Python's current directory through its parents. This keeps paths
in Unicode, works from a nested directory and avoids passing a path through a
native command's stdout. In particular, Windows PowerShell can decode the UTF-8
output of `git rev-parse` with its OEM code page, turning a Cyrillic username into
a different path before Python starts. A successful probe in a terminal with a
UTF-8 profile does not establish that Codex's hook runner can launch that command.

For this repository, `russian-wording` arrives at session start. The Go, HTTP, gateway and UI topic rules are path-scoped and attach on demand. An edit touching `external/httpserver/server.go` pulls in `api-layer`, `architecture`, `code-style`, `implementation-order`, `testing`, `workflow` and `upstream-divergences`; one touching `external/ui/src/` also pulls in `ui-spa` and `ui-verification`.

## How it works

The project adapter parses `.mdc` frontmatter itself (`description`, scalar or list `globs`, and `alwaysApply`). Quoted commas, doubled single quotes, double-quoted escapes and YAML comments in valid scalar, single-line or multiline flow-list, and block-list forms are decoded or stripped as appropriate. It reads `.cursor/rules/` as its input and keeps no separate Codex copy of a rule body. A new Cursor rule file needs no adapter wiring. Points worth knowing:

- **Glob matching is hand-rolled.** `fnmatch` is unusable because its `*` also crosses `/`, which makes `external/httpserver/**/*.go` miss `external/httpserver/server.go`. The script translates globs to a regex where `**/` becomes "zero or more directories", `*` stays inside one segment, and a bare `**` spans anything.
- **Paths come from patch headers and structured tool fields.** `*** Add File:`, `*** Update File:`, `*** Delete File:` and `*** Move to:` are read, as are known JSON fields such as `file_path`, `path`, `source` and `destination`. Absolute paths are made repo-relative, and paths outside the repository are ignored.
- **Each scoped rule is injected at most once per active session context.** State is a JSON file under `<tempdir>/codex-attach-rules-<repository-hash>/<session_id>.json`, so clones cannot suppress one another. `resume` preserves scoped dedupe; `startup`, `compact` and `clear` reset state to the always-on set so scoped rules can attach again when needed. Concurrent processes serialize delivery with a per-session lock, and state updates use atomic replacement. Rule ids are persisted only after context output succeeds, so failed output leaves them eligible for a later attempt. If locking or persistence is unavailable, matching rule context is still emitted and deduplication may temporarily degrade to duplicate delivery.
- **It fails open.** Malformed JSON on stdin, an unparsable rule file, or an unwritable state directory all exit 0 with no output. A broken rule can never block an edit.

## Context budget

Codex caps model-visible hook output at roughly 2500 tokens by default, then **spills** the remainder to `<temp_dir>/hook_outputs/` and shows the model a head-and-tail preview instead. Both handlers therefore raise `additionalContextLimit`, to 8000 at session start and 6000 per patch. Those are ceilings, not reservations; the real payloads sit well under them.

Keep this in mind when adding rules. The budget is shared with every other hook and plugin in the session, and oversized always-on context degrades the model rather than helping it.

## Trust

Codex requires explicit approval before a non-managed command hook can run, and records that approval against the **hash of the hook definition**. An unapproved hook is skipped without a hard error, which looks exactly like a hook that is not firing.

- Run `/hooks` once per clone to review and trust both handlers.
- Run it again after **any** edit to `attach_rules.py` or `hooks.json`.
- Project-local hooks load only when the `.codex/` layer is trusted. In an untrusted checkout Codex still loads user and system hooks, but not these.

This mirrors the workspace trust gate FoxxyCode applies to project-local `.foxxycode/mcp.json` (see [MCP Integration](../features/mcp.md)): opening a repository must not by itself grant it code execution.

## Verifying and debugging

Run the repository adapter contract first:

```bash
make test-agent-rules
```

It covers scalar and YAML-list `globs`, structured edit paths, provider-rule activation, OpenCode delivery and the absence of a manual Codex index.

Drive the script by hand with a synthetic payload. It reads JSON on stdin and writes JSON on stdout, so no Codex session is needed. In Git Bash on Windows, replace `python3` with `python` in these probes:

```bash
echo '{"hook_event_name":"PreToolUse","session_id":"probe","tool_input":{"command":"*** Begin Patch\n*** Update File: external/httpserver/server.go\n*** End Patch"}}' | python3 .codex/hooks/attach_rules.py
```

Session-start behaviour:

```bash
echo '{"hook_event_name":"SessionStart","session_id":"probe","source":"startup"}' | python3 .codex/hooks/attach_rules.py
```

Empty output is a valid answer and means one of three things: no glob matched, the rule was already sent in this session, or the input was not understood. Clear the dedup state to re-test:

```bash
rm -rf "${TMPDIR:-/tmp}"/codex-attach-rules-*
```

If a hook produces nothing inside a real session, check `/hooks` first. Trust is the usual cause.

The hook statistics distinguish a skipped hook from a failed launch. If a row says
`hook exited with code 1`, the hook ran and failed; approving it again does not fix
the command. Expand the row to inspect its output. The old Windows launcher could
fail with this generic error before reaching `attach_rules.py`, even though
running that script directly succeeded. Update both Windows commands to the
launcher in this checkout, review the changed definitions in `/hooks`, and resume
or restart the session so `SessionStart` can deliver its always-on rules again.

Run the launcher regression checks on Windows:

```powershell
python -X utf8 scripts/test_codex_hooks.py
```

They run the configured commands through both `cmd.exe` and Windows PowerShell
with OEM decoding, in a temporary checkout with spaces and Cyrillic in its path.
They check session-start rules, a scoped rule from a nested directory with a
Unicode absolute file path, duplicate suppression and the `fork` start matcher.

## Adding or changing a rule

Edit `.cursor/rules/*.mdc` as before. The hook needs no change when a topic is added or renamed because it reads the directory on each event.

- a rule is reachable from Codex only when its frontmatter carries `globs` or `alwaysApply: true`;
- update the deliberate Claude Code counterpart in the same change and keep the bodies equivalent;
- run `make test-agent-rules` after changing rule frontmatter or either adapter file.

## Not to be confused with Codex `.rules`

Codex uses the word "rules" for a second, unrelated mechanism: `.rules` files written in Starlark under a `rules/` directory next to an active config layer, which decide **which shell commands may run outside the sandbox**. Those are an execution policy, not instructions, and they are validated with:

```bash
codex execpolicy check --pretty --rules ~/.codex/rules/default.rules -- go test ./...
```

This repository ships no `.codex/rules/` directory; command policy is a per-developer concern and lives in `~/.codex/rules/`. Note that Codex only decomposes a `bash -lc` script into separate commands when it is a plain chain of words joined by `&&`, `||`, `;` or `|`. A script containing `VAR=x`, `$(...)`, a wildcard, or control flow is matched as one opaque invocation, which is why auto-recorded allow-list entries tend to be unusable on any later command.

## Equivalent setups in other agents

| Agent | Always-on rules | Scoped rules |
|---|---|---|
| Cursor | `alwaysApply: true` in `.cursor/rules/*.mdc` | `globs` in the same frontmatter |
| Claude Code | `CLAUDE.md` (symlinked to `AGENTS.md` here) | native `.claude/rules/*.md` path scoping |
| Codex | `AGENTS.md` plus the `SessionStart` hook | `PreToolUse` hook, this page |
| ZCode | [workspace hook config](zcode-hooks.md) | `PreToolUse` hook over JSON path fields |
| Kimi Code | [user-level hook config](kimi-code-hooks.md) | `PreToolUse` hook over JSON path fields |

Upstream reference: [Hooks](https://developers.openai.com/codex/hooks) and [Custom instructions with AGENTS.md](https://developers.openai.com/codex/guides/agents-md).
