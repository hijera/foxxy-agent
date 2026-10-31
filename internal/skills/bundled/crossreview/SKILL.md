---
name: crossreview
metadata:
  version: 1.0.0
description: >
  Run when the user invokes /crossreview or asks for a quorum review: fan a code review out
  to several external console code agents (claude, codex, foxxycode, opencode, cursor, devin, koda)
  and/or internal reviewers, then merge the findings into one verdict. First run detects the
  installed CLIs and asks which agents and models to use; the roster is stored for reuse.
---

# Crossreview — quorum code review

Drives the bundled `crossreview` subagent, which orchestrates the actual fan-out. This skill
is the interactive front-end: detection, the questions about agents and models, and writing the
roster happen here, because a subagent cannot ask the user questions.

**Agent mode required.** Setup writes files and the coordinator spawns and shells out; in plan
or ask mode none of that works. If the session is not in agent mode, tell the user to switch to
it first and stop.

## Step 1 — the roster

The roster lists the reviewers. Read `${FOXXYCODE_HOME}/crossreview.json` first — it is the
preferred location. If it is absent, look for `.foxxycode/crossreview.json` in the workspace.

A **workspace** roster is executable configuration that arrived with the checkout, so it is
honoured only after the user approves exactly this file: compute its sha256, check
`${FOXXYCODE_HOME}/crossreview-trust.json` for an entry with this workspace's canonical path and
that digest. Without one, show the user the reviewer commands the file declares and `question`
them whether to trust it; on approval append `{"workspace": "<canonical path>", "sha256":
"<digest>"}` to that JSON file (create it if needed). On refusal, ignore the workspace roster.

If the roster is missing entirely, or the user asked `/crossreview setup`, run setup:

1. Run `detect-agents.sh` next to this SKILL.md (`run_command`, `sh <skilldir>/detect-agents.sh`;
   on Windows use `detect-agents.ps1` via `powershell -File`). If the script is gone, fall back
   to probing `command -v` (or `where`) for `claude codex foxxycode opencode cursor-agent agent cursor
   devin koda` yourself. The output is one line per detected agent:
   `agent <TAB> binary_path <TAB> models_cmd <TAB> run_template`.
2. `question` with `multiple: true` — which **detected** agents to use as reviewers. Never offer
   an agent the detection did not find.
3. For each chosen agent whose `run_template` contains a `{model}` placeholder, pick the
   models. When the line carries a `models_cmd`, run it (`run_command`) and offer its output
   as `question` options (`multiple: true`). When it does not — claude and foxxycode (whose models
   live in `models[].model` of its config; read them with `config_get models` when the user
   wants the local foxxycode as a reviewer) — ask with `custom: true`; even a custom question
   needs at least one option, so offer "Enter model id manually". One reviewer entry per
   (agent, model) pair the user picks. An agent whose `run_template` has no `{model}`
   placeholder — koda's default is `koda "$(cat {brief})" > {out}` — contributes one reviewer
   entry with no model question; if that CLI takes a model flag, the user edits it into the
   stored `command` by hand.
4. Write the roster to `${FOXXYCODE_HOME}/crossreview.json`:

   ```json
   {"version": 1, "min_reviewers": 2, "reviewers": [
     {"kind": "cli", "agent": "codex", "binary": "codex", "model": "gpt-5.6-luna",
      "command": "codex exec -m gpt-5.6-luna --sandbox read-only - < {brief} > {out}",
      "prompt_via": "stdin"},
     {"kind": "internal", "definition": "explore",
      "model": "vendor/model-id", "reasoning": "off"}
   ]}
   ```

   Substitute the chosen model into the template's `{model}` placeholder so the stored
   `command` is fully resolved — the coordinator only replaces `{brief}` and `{out}`. `kind:
   "internal"` entries run as `explore` children of the coordinator on a model FoxxyCode already
   knows; set `reasoning` only when the user picked a level. `min_reviewers` defaults to 2 —
   raise it if the user asks for a stricter quorum. To also use the workspace roster location,
   write `.foxxycode/crossreview.json` instead (it will ask for approval on the next run).

## Step 2 — the review

1. Resolve the scope from the user's message: a git ref range, a file list, a document or plan
   path — whatever they named. Default: the uncommitted work in progress (`git diff HEAD` plus
   untracked files) in the current workspace.
2. `spawn_agent` with `agent: "crossreview"` (foreground — do not detach it). The prompt names
   the scope, the roster path, and the skill directory (the coordinator finds
   `detect-agents.sh` there when no roster exists).
3. When it returns, restate the verdict and the deduplicated findings in your reply — the
   coordinator's final message is the whole review.
