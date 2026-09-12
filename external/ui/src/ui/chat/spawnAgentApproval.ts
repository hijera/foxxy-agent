/**
 * Recognising a `spawn_agent` call that was refused for want of an approval.
 *
 * Nothing here reads the refusal text. The trigger is structural — a failed
 * tool call named `spawn_agent` — and the agent name comes out of the call's
 * own arguments, which the model wrote as JSON. Whether that definition is
 * actually awaiting approval is then decided by the catalog, never by parsing
 * prose that changes with the wording or the locale.
 */

export type SpawnAgentCallShape = {
  title?: string | undefined;
  kind?: string | undefined;
  status?: string | undefined;
  argsText?: string | undefined;
};

const SPAWN_TOOL = "spawn_agent";

function names(call: SpawnAgentCallShape): string[] {
  return [(call.title ?? "").trim().toLowerCase(), (call.kind ?? "").trim().toLowerCase()];
}

/** True for a `spawn_agent` tool call the runtime rejected. */
export function isSpawnAgentRefusal(call: SpawnAgentCallShape): boolean {
  if (!names(call).includes(SPAWN_TOOL)) {
    return false;
  }
  return (call.status ?? "").trim().toLowerCase() === "failed";
}

/**
 * The `agent` argument of a spawn call, or "" when the arguments are missing,
 * truncated or not an object — a streamed call can be rendered before its
 * arguments are complete.
 */
export function parseSpawnAgentName(argsText: string | undefined): string {
  const raw = (argsText ?? "").trim();
  if (!raw) {
    return "";
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return "";
    }
    const value = (parsed as Record<string, unknown>)["agent"];
    return typeof value === "string" ? value.trim() : "";
  } catch {
    return "";
  }
}

/** The definition name to offer an approval for, or "" when there is none. */
export function refusedSpawnAgentName(call: SpawnAgentCallShape): string {
  return isSpawnAgentRefusal(call) ? parseSpawnAgentName(call.argsText) : "";
}
