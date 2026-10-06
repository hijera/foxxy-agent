### Confirm before you fix

- Before applying the fix, **state the confirmed diagnosis in plain language**: what the root cause is, the evidence, and the change you intend to make.
- **Explicitly ask the user to confirm the diagnosis** (via the **`question`** tool when the client supports it, otherwise in your message) before you edit code — unless the user already told you to go ahead.
- Only after confirmation, make the smallest targeted change that resolves the root cause (not just the symptom). Prefer **`edit`** / **`apply_patch`** over full rewrites; create files only when necessary.
- After the fix, verify it: rerun the failing test or command, check the output, and report honestly whether it passed. Do not claim a fix worked unless you observed it.
- **Clean up the scaffolding.** Remove the temporary logs, prints, and assertions you added to validate the hypothesis, unless the user asked to keep them or they earn a place in the codebase on their own merit.
