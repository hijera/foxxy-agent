# Skills

## Overview

Skills are reusable instruction packs discovered from **`skills.dirs`** in **`config.yaml`**. They power slash commands, the **`{{.Skills}}`** block in the system prompt, and ACP **`available_commands_update`** notifications.

**Project rules** (**.cursor/rules**, **.coddy/rules**, etc.) are a **separate** mechanism. They are auto-discovered under the session **cwd** and injected as **`{{.Rules}}`**. See **[rules.md](rules.md)** - do not rely on **`skills.dirs`** to load **`.cursor/rules/`**.

## Supported file types

### Agent skills (`SKILL.md`)

Skill files provide reusable instructions for specific tasks. Compatible with the Cursor skills layout (`subdir/SKILL.md`).

Format:

```markdown
# Skill Title

Short description of what this skill does.

## Instructions

Detailed instructions...
```

Skills are discovered by searching for **`SKILL.md`** under each configured directory. A **symbolic link** in a skill root that points to a directory is treated like a normal subfolder when that directory contains **`SKILL.md`**.

### Root `.md` / `.mdc` in skill directories

Optional markdown at the **root** of a **`skills.dirs`** entry (not under **`.cursor/rules/`**) may register as slash skills. **Project rules** belong under **`.coddy/rules/`**, **`.cursor/rules/`**, and the other trees in **[rules.md](rules.md)** - not in **`skills.dirs`**.

## Loading priority

Directories are scanned in config order (see **`skills.dirs`** in [config.md](config.md)). Built-in defaults when **`dirs`** is empty:

1. **`${CODDY_HOME}/skills/`** - installed skills (agent home)
2. **`${CWD}/.skills/`** - project skills (session working directory)
3. **`~/.cursor/skills/`**
4. **`~/.claude/skills/`**

Bundled **`/coddy-generate-rules`** is always prepended (writes **`.coddy/rules/*.mdc`** by default).

## Slash commands catalog

Every discovered skill has a canonical slash **`name`** (folder name for `subdir/SKILL.md`, file stem for root `*.md` / `*.mdc`). The agent builds one **`Skills`** template block:

1. A Markdown catalog listing all commands with short descriptions (**`ListSkills`**).
2. Full bodies for **`globs`** / **`alwaysApply`** matches (skills pipeline).
3. On a user message, **`/name`** tokens preceded by line start or ASCII whitespace (and legacy **`[/name](coddy-skill:name)`** forms if present in stored text) append the matching skill body for that turn when the name is **not** already in the glob-selected active set.

ACP clients receive **`session/update`** **`available_commands_update`** after **`session/new`** and **`session/load`** (see **`examples/acp/acp_e2e_skills_slash.py`**).

The **`coddy http`** SPA queries **`GET /coddy/slash-commands`** for autocomplete.

## How skills are applied

On **`session/prompt`**, the agent:

1. Loads skills from **`skills.dirs`** for the session **cwd** and **`CODDY_HOME`**
2. Applies glob / **`alwaysApply`** filtering for the skills section
3. Merges slash catalog plus invoked **`/name`** bodies into **`{{.Skills}}`**

Rules from **`internal/rules`** are merged separately into **`{{.Rules}}`**.

## Example skill file

`~/.cursor/skills/code-review/SKILL.md`:

```markdown
# Code Review Skill

Provides guidance for conducting thorough code reviews.

## Instructions

When asked to review code:
1. Check for security vulnerabilities
2. Verify error handling is complete
3. Look for performance issues
4. Check test coverage
5. Verify documentation is adequate
```

## Adding custom skills at runtime

Configure extra directories in **`config.yaml`**:

```yaml
skills:
  dirs:
    - "~/my-custom-skills"
    - "/shared/team-skills"
```

Or install into **`skills.install_dir`** via CLI:

- **`coddy skills list`**
- **`coddy skills install <path-or-url>`**
- **`coddy skills uninstall <name>`**

## References

- Implementation: **`internal/skills/*`**, wiring in **`internal/session`**, **`internal/agent/system_prompt.go`**
