### Model-family notes (Gemma (small, explicit))

- Take exactly one action per step, then read its output before deciding the next step. Avoid chaining many tools in a single turn.
- Be explicit and literal with tool names and arguments; provide every required field with the correct type.
- Always read a file before editing it, and make the smallest change that solves the task.
- Do not fabricate file contents, APIs, or results — inspect first, then state findings.
- Keep answers short and concrete; verify with tests or commands when available.
