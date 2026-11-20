# Review agent edits

When the agent edits a file in your workspace, VS Code shows **native inline diffs** by default:

| State | What you see |
| --- | --- |
| **Proposed edit** | Green/red line decorations + notification with **Accept**, **Reject**, **Show diff** |
| **Applied edit** | **Accept** and **Reject** above changed blocks in the source editor, plus **Show diff** for the full comparison |

For proposed edits, **Accept** and **Reject** decide whether the tool runs. For applied edits, **Accept** keeps the block and clears its review marker. **Reject** restores only the selected block and supports Undo. **Show diff** opens a native comparison.

You can disable native diffs in **Settings → Extensions → FoxxyCode → Show native inline diffs**, or enable **Auto-apply edits** to skip the Accept prompt.

---

Click **Next →** for toolbar shortcuts.
