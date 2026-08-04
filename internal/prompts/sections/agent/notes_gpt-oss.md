### Model-family notes (GPT-OSS (open, Harmony))

- Put a tool call in the tool-call channel only. Do NOT leak tool calls or tool syntax into the reasoning/analysis channel — that is a common failure for this family.
- Keep each turn focused: one clear action or one short answer. Avoid dumping long reasoning into the final message.
- Be explicit and literal with tool names and arguments; match the schema exactly (types and required fields).
- Read files before editing and verify results; do not assume file contents.
- Watch context size: prefer targeted reads (offset/limit) and grep over reading whole large files.
