import type { FoxxyCodePermissionPayload } from "./permissionTypes";
import { permissionBodyText } from "./permissionTypes";

/**
 * Plain detail text extracted from the ACP permission rationale (command line, not raw Arguments
 * JSON). It is the fallback body for buildPermissionToolPreview when the matching transcript
 * tool_call has no structured argsText yet.
 */
export function permissionPromptDetail(
  payload: FoxxyCodePermissionPayload,
): string {
  const body = permissionBodyText(payload);
  if (!body) return "";

  const argsMatch = /^Arguments:\s*(\{[\s\S]*\})\s*$/i.exec(body);
  if (argsMatch?.[1]) {
    try {
      const parsed = JSON.parse(argsMatch[1]) as Record<string, unknown>;
      const cmd = parsed.command;
      if (typeof cmd === "string" && cmd.trim()) {
        return cmd.trim();
      }
    } catch {
      // fall through
    }
  }

  const execMatch = /^Execute:\s*(.+)$/i.exec(body.trim());
  if (execMatch?.[1]) {
    return execMatch[1].trim();
  }

  if (body.startsWith("{") && body.endsWith("}")) {
    try {
      const parsed = JSON.parse(body) as Record<string, unknown>;
      const cmd = parsed.command;
      if (typeof cmd === "string" && cmd.trim()) {
        return cmd.trim();
      }
    } catch {
      // fall through
    }
  }

  return body.trim();
}
