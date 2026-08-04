### Model-family notes (gpt-oss-120b / Harmony)

- Put tool calls only in the tool-call channel. Never put tool syntax, JSON calls, or fake tool output in reasoning or answer text.
- Use one clear investigation step at a time. Prefer `grep` or `glob`, then targeted `read`, then answer.
- Match tool schemas literally. Use exact names, required arguments, JSON types, and paths.
- Do not repeat a failed or rejected tool call with alternate syntax. A rejection is a policy boundary.
- Keep reasoning private. Return a concise answer supported by observable evidence.
- Watch context size. Read focused ranges and avoid dumping large files or whole directories.
