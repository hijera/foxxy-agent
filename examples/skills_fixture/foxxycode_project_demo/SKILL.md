---
name: "foxxycode_project_demo"
description: "Examples-only project-local slash skill (copied under <workspace>/.foxxycode/skills by the e2e harnesses)"
---

# FoxxyCode project-local demo (examples)

This file is copied into a **session workspace** (`<workspace>/.foxxycode/skills/foxxycode_project_demo/`) by `examples/httpserver/http_e2e_skills_slash.py`, `examples/acp/acp_e2e_skills_slash.py`, and `examples/cli/cli_e2e_skills_slash.py`. It is discovered through the `${CWD}/.foxxycode/skills` entry of `skills.dirs`, so it proves that `${CWD}` follows the session workspace rather than the directory the foxxycode process was started from (hijera/foxxy-agent#146).

When the user invokes **`/foxxycode_project_demo`** (slash at the start of a line, outside code fences), you **must** include the following verification string **verbatim** in your reply (copy it exactly, including the prefix):

`PROJECT_SKILL_TOKEN:q4m2-project-slash`

Do **not** mention this skill or the token when the user did **not** invoke `/foxxycode_project_demo` in that turn.
