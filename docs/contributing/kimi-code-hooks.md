# Kimi Code hooks for project rules

This page is for contributors running Kimi Code against this repository. It is the Kimi Code counterpart of [Codex hooks](codex-hooks.md) and [ZCode hooks](zcode-hooks.md): all three deliver the same `.cursor/rules/*.mdc` files through the client's lifecycle hooks.

## Why a hook is needed

Kimi Code reads the repository's `AGENTS.md`, but it does not interpret the `globs` and `alwaysApply` fields in `.cursor/rules/*.mdc`. Asking the model to open the relevant rule before every edit is advisory and can be skipped.

`.kimi-code/hooks/attach_rules.py` reads that frontmatter directly and injects the rule bodies as hook context. `.cursor/rules/` remains the single source of truth; adding or changing a rule needs no hook wiring.

## What fires when

| Event | Matcher | What is injected |
|---|---|---|
| `SessionStart` | `startup\|resume` | every rule whose frontmatter has `alwaysApply: true` |
| `PreToolUse` | `^(Edit\|Write)$` | rules whose `globs` cover a file in the pending tool call |

Both handlers run the same script and branch on `hook_event_name` from the JSON payload. Kimi Code passes `Edit` and `Write` paths in `tool_input.path`; the script also recognizes common nested path fields so the payload can evolve without changing the matcher.

## Enabling

Kimi Code currently reads hook declarations from the **user-level** `~/.kimi-code/config.toml` (or `$KIMI_CODE_HOME/config.toml`). It does not automatically load a project-local hook configuration. The repository therefore ships `.kimi-code/hooks.toml` as a fragment to merge, not as an automatically active config file.

Append the two `[[hooks]]` entries from `.kimi-code/hooks.toml` to the user-level `config.toml`, preserving the hooks already there, and start a new Kimi Code session. Do not replace the whole config with the fragment. The declarations are global, while their command uses a path relative to the current project directory; in a checkout without `.kimi-code/hooks/attach_rules.py`, the command cannot attach rules and Kimi Code's fail-open behaviour keeps the session usable.

The command invokes `python`, not `python3`, because Windows may resolve `python3` to the Microsoft Store stub instead of the installed interpreter. Python 3 must be on `PATH`; on a Unix-like system that only provides `python3`, change the command in the user-level config accordingly.

## How it works

- **Glob matching is hand-rolled.** `**/` means zero or more directories, `*` stays inside one path segment, and a bare `**` spans anything. Thus `external/httpserver/**/*.go` matches both `external/httpserver/server.go` and deeper files.
- **Paths are normalised before matching.** Absolute paths inside the checkout become repo-relative; absolute paths outside it are ignored.
- **Each rule is injected at most once per session.** State is a JSON file under `<tempdir>/kimi-code-foxxycode-rules/<session_id>.json`, so repeated edits do not resend the same rule. A new or resumed `SessionStart` reseeds the state with the always-on rules.
- **It fails open.** Malformed JSON, an unreadable rule, or an unwritable state directory produces no blocking response. The hook is an instruction-delivery mechanism, not a security boundary.

## Verifying and debugging

The unit suite uses temporary rule and state directories and needs no Kimi Code session, provider, or third-party Python package:

```bash
python -m unittest discover -s .kimi-code/tests -v
```

Drive the real script with a synthetic Kimi Code payload:

```bash
echo '{"hook_event_name":"SessionStart","session_id":"kimi-probe","source":"startup"}' | python .kimi-code/hooks/attach_rules.py
echo '{"hook_event_name":"PreToolUse","session_id":"kimi-edit-probe","tool_name":"Edit","tool_input":{"path":"external/httpserver/server.go"}}' | python .kimi-code/hooks/attach_rules.py
```

A successful match prints a JSON object containing `hookSpecificOutput.additionalContext`. Empty output with exit code 0 means no glob matched, the rule was already sent in that session, or the input was not understood. Use a fresh session id for another probe, set `KIMI_CODE_RULES_STATE_DIR` to a throwaway directory, or remove `<tempdir>/kimi-code-foxxycode-rules` to clear dedup state. `KIMI_CODE_RULES_DIR` can point at a fixture rule tree for isolated tests.

After changing the user-level config, verify in a **new** Kimi Code session rather than assuming an existing session has reloaded its hooks.

## Relationship to the other adapters

This script intentionally follows `.zcode/hooks/attach_rules.py`: both receive edited paths as JSON fields rather than parsing Codex `apply_patch` headers. Keep shared parsing, matching, and state behaviour aligned when changing the adapters, and test each changed script independently. OpenCode uses a plugin instead; see [OpenCode hooks](opencode-hooks.md).

Upstream references: [Kimi Code hooks](https://www.kimi.com/code/docs/en/kimi-code-cli/customization/hooks.html) and [configuration files](https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/config-files.html).
