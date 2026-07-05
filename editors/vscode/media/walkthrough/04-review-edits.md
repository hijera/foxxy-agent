# Review agent edits

When the agent edits a file in your workspace, VS Code shows **native inline diffs** by default:

| State | What you see |
| --- | --- |
| **Proposed edit** | Green/red line decorations + notification with **Accept**, **Reject**, **Show diff** |
| **Applied edit** | Decorations + **Revert** and **Show diff** |

**Accept** / **Reject** posts your decision to the foxxycode server. **Show diff** opens a side-by-side diff editor. **Revert** writes the pre-edit content back.

You can disable native diffs in **Settings → Extensions → FoxxyCode → Show native inline diffs**, or enable **Auto-apply edits** to skip the Accept prompt.

---

Click **Next →** for toolbar shortcuts.
