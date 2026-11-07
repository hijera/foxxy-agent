#!/usr/bin/env python3
"""Attach `.cursor/rules/*.mdc` to Kimi Code sessions deterministically.

Kimi Code loads AGENTS.md but does not attach Cursor rules by their `globs`.
This hook restores that behaviour by reading the rule frontmatter directly:

  SessionStart -> inject every rule with `alwaysApply: true`
  PreToolUse   -> inject rules whose `globs` match the file paths an Edit or
                  Write call is about to touch, at most once per session

`.cursor/rules/` stays the single source of truth; nothing is duplicated here.
The hook fails open: unexpected input or an unreadable rule never blocks an edit.

Kimi Code reads hook declarations from the user-level config.toml. The versioned
fragment to merge into that file is `.kimi-code/hooks.toml`.
"""

from __future__ import annotations

import json
import os
import re
import sys
import tempfile
from pathlib import Path

SCRIPT_ROOT = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_ROOT.parents[1]
RULES_DIR = Path(os.environ.get("KIMI_CODE_RULES_DIR") or (REPO_ROOT / ".cursor" / "rules"))
STATE_DIR = Path(
    os.environ.get("KIMI_CODE_RULES_STATE_DIR")
    or (Path(tempfile.gettempdir()) / "kimi-code-foxxycode-rules")
)

PATH_FIELDS = frozenset(
    {
        "file_path",
        "path",
        "notebook_path",
        "source",
        "destination",
        "new_path",
        "old_path",
        "old_string_path",
        "filePath",
        "fileName",
        "directory",
    }
)


class Rule:
    def __init__(self, path: Path, description: str, globs: list[str], always: bool, body: str):
        self.path = path
        self.description = description
        self.globs = globs
        self.always = always
        self.body = body

    @property
    def rel(self) -> str:
        try:
            return self.path.relative_to(REPO_ROOT).as_posix()
        except ValueError:
            return self.path.as_posix()


def glob_to_regex(pattern: str) -> re.Pattern[str]:
    """Translate a Cursor glob into a regex without letting `*` cross `/`."""
    out: list[str] = []
    i, n = 0, len(pattern)
    while i < n:
        if pattern.startswith("**/", i):
            out.append("(?:.*/)?")
            i += 3
        elif pattern.startswith("**", i):
            out.append(".*")
            i += 2
        elif pattern[i] == "*":
            out.append("[^/]*")
            i += 1
        elif pattern[i] == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(pattern[i]))
            i += 1
    return re.compile("^" + "".join(out) + "$")


def parse_rule(path: Path) -> Rule | None:
    """Read one `.mdc` file. Frontmatter is flat, so no YAML dependency."""
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---"):
        return None
    end = text.find("\n---", 3)
    if end < 0:
        return None
    head = text[3:end]
    body = text[end + 4 :].strip()

    description, globs, always = "", [], False
    for line in head.splitlines():
        key, sep, value = line.partition(":")
        if not sep:
            continue
        key, value = key.strip(), value.strip()
        if key == "description":
            description = value
        elif key == "globs":
            globs = [g.strip() for g in value.split(",") if g.strip()]
        elif key == "alwaysApply":
            always = value.lower() == "true"
    return Rule(path, description, globs, always, body)


def load_rules() -> list[Rule]:
    rules = []
    for path in sorted(RULES_DIR.glob("*.mdc")):
        rule = parse_rule(path)
        if rule and rule.body:
            rules.append(rule)
    return rules


def state_file(session_id: str) -> Path:
    safe = re.sub(r"[^A-Za-z0-9._-]", "_", session_id or "unknown")[:120]
    return STATE_DIR / f"{safe}.json"


def load_sent(session_id: str) -> set[str]:
    try:
        return set(json.loads(state_file(session_id).read_text(encoding="utf-8")))
    except Exception:
        return set()


def save_sent(session_id: str, sent: set[str]) -> None:
    try:
        STATE_DIR.mkdir(parents=True, exist_ok=True)
        state_file(session_id).write_text(json.dumps(sorted(sent)), encoding="utf-8")
    except Exception:
        pass


def collect_target_paths(tool_input: object) -> list[str]:
    """Collect repo-relative paths from known fields in a Kimi tool payload."""
    found: list[str] = []

    def walk(node: object) -> None:
        if isinstance(node, dict):
            for key, value in node.items():
                if key in PATH_FIELDS and isinstance(value, str) and value:
                    found.append(value)
                else:
                    walk(value)
        elif isinstance(node, list):
            for item in node:
                walk(item)

    walk(tool_input)

    rel: list[str] = []
    for raw in found:
        path = Path(raw)
        if path.is_absolute():
            try:
                path = path.relative_to(REPO_ROOT)
            except ValueError:
                continue
        rel.append(path.as_posix())
    return rel


def render(rules: list[Rule], preamble: str) -> str:
    chunks = [preamble]
    for rule in rules:
        chunks.append(f"<!-- from {rule.rel} -->\n\n{rule.body}")
    return "\n\n".join(chunks)


def emit(event: str, context: str) -> None:
    json.dump(
        {"hookSpecificOutput": {"hookEventName": event, "additionalContext": context}},
        sys.stdout,
    )


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except Exception:
        return 0

    event = payload.get("hook_event_name", "")
    session_id = payload.get("session_id", "")

    try:
        rules = load_rules()
    except Exception:
        return 0
    if not rules:
        return 0

    if event == "SessionStart":
        always = [r for r in rules if r.always]
        if not always:
            return 0
        save_sent(session_id, {r.rel for r in always})
        emit(
            event,
            render(
                always,
                "Project rules for this repository, always in force. They are"
                " authoritative; `.cursor/rules/` is their single source of truth."
                " More rules are attached automatically when you edit files they"
                " cover.",
            ),
        )
        return 0

    if event != "PreToolUse":
        return 0

    paths = collect_target_paths(payload.get("tool_input"))
    if not paths:
        return 0

    sent = load_sent(session_id)
    matched: list[Rule] = []
    for rule in rules:
        if rule.always or rule.rel in sent or not rule.globs:
            continue
        patterns = [glob_to_regex(g) for g in rule.globs]
        if any(p.match(path) for path in paths for p in patterns):
            matched.append(rule)

    if not matched:
        return 0

    save_sent(session_id, sent | {r.rel for r in matched})
    touched = ", ".join(sorted(set(paths))[:8])
    emit(
        event,
        render(
            matched,
            f"Project rules that cover the files you are editing ({touched})."
            " Apply them to this change before continuing.",
        ),
    )
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        sys.exit(0)
