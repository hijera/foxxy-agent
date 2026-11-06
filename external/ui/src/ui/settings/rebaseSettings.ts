function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

/** Replay edits made since the request over its saved result. Lists, like the
 * server's config merge, are atomic values; objects merge by field. */
function replay(before: unknown, edited: unknown, saved: unknown): unknown {
  if (JSON.stringify(before) === JSON.stringify(edited)) return saved;
  if (!isObject(before) || !isObject(edited) || !isObject(saved)) return edited;
  const result = { ...saved };
  for (const key of new Set([...Object.keys(before), ...Object.keys(edited)])) {
    if (!(key in edited)) {
      delete result[key];
    } else {
      result[key] = replay(before[key], edited[key], saved[key]);
    }
  }
  return result;
}

export function rebaseSettings(
  sent: Record<string, unknown>,
  edited: Record<string, unknown>,
  saved: Record<string, unknown>,
): Record<string, unknown> {
  const result = replay(sent, edited, saved) as Record<string, unknown>;
  return { ...result, revision: saved.revision };
}
