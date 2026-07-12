// Serialize / deserialize an LLM provider entry for sharing between installs.
//
// Secrets are never transferred: api_key, api_key_command and proxy are stripped
// on export and defensively dropped on import. Only the safe descriptive fields
// (name, type, api_base) travel — matching the provider schema in
// internal/config/ui_schema.go (providerProps).

/** Fields that carry secrets / host-specific wiring and must never be shared. */
export const PROVIDER_TRANSFER_OMIT = [
  "api_key",
  "api_key_command",
  "proxy",
] as const;

/** Provider fields that are safe to export/import. */
export const PROVIDER_TRANSFER_KEYS = ["name", "type", "api_base"] as const;

/** Custom scheme prefix used for the clipboard/query-string form. */
export const PROVIDER_TRANSFER_SCHEME = "foxxycode://provider";

type ProviderLike = Record<string, unknown>;

function stringField(v: unknown): string {
  return v === undefined || v === null ? "" : String(v);
}

/** Keep only the safe, non-empty transfer fields. */
export function sanitizeProvider(p: ProviderLike): Record<string, string> {
  const out: Record<string, string> = {};
  for (const k of PROVIDER_TRANSFER_KEYS) {
    const s = stringField(p[k]).trim();
    if (s !== "") {
      out[k] = s;
    }
  }
  return out;
}

/** Pretty JSON for the file export (single provider object, secrets stripped). */
export function providerToJson(p: ProviderLike): string {
  return JSON.stringify(sanitizeProvider(p), null, 2);
}

/** Clipboard form: `foxxycode://provider?name=…&type=…&api_base=…` (secrets stripped). */
export function providerToClipboard(p: ProviderLike): string {
  const params = new URLSearchParams();
  const safe = sanitizeProvider(p);
  for (const k of PROVIDER_TRANSFER_KEYS) {
    if (safe[k] !== undefined) {
      params.set(k, safe[k]);
    }
  }
  return `${PROVIDER_TRANSFER_SCHEME}?${params.toString()}`;
}

function fromSearchParams(params: URLSearchParams): Record<string, string> {
  const out: Record<string, string> = {};
  for (const k of PROVIDER_TRANSFER_KEYS) {
    const v = params.get(k);
    if (v !== null && v.trim() !== "") {
      out[k] = v.trim();
    }
  }
  return out;
}

function parseQueryLike(text: string): Record<string, string> {
  // Accept a full URL (foxxycode://provider?…), a bare query string (name=…&…),
  // or a leading "?query". Avoid URL.canParse (banned by the Chromium 104 baseline).
  if (text.includes("://")) {
    try {
      const u = new URL(text);
      return fromSearchParams(u.searchParams);
    } catch {
      // fall through to raw query parsing
    }
  }
  const q = text.startsWith("?") ? text.slice(1) : text;
  const at = q.indexOf("?");
  const query = at >= 0 ? q.slice(at + 1) : q;
  return fromSearchParams(new URLSearchParams(query));
}

function normalizeJson(parsed: unknown): Record<string, string>[] {
  const rows = Array.isArray(parsed) ? parsed : [parsed];
  const out: Record<string, string>[] = [];
  for (const row of rows) {
    if (row && typeof row === "object" && !Array.isArray(row)) {
      const safe = sanitizeProvider(row as ProviderLike);
      if (Object.keys(safe).length > 0) {
        out.push(safe);
      }
    }
  }
  return out;
}

/**
 * Parse pasted clipboard text or a JSON file's contents into provider entries.
 * Auto-detects JSON (object/array) vs query/URL form. Returns an empty array
 * when nothing usable was found; throws only on malformed JSON.
 */
export function parseProviderTransfer(text: string): Record<string, string>[] {
  const trimmed = text.trim();
  if (trimmed === "") {
    return [];
  }
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    return normalizeJson(JSON.parse(trimmed));
  }
  const one = parseQueryLike(trimmed);
  return Object.keys(one).length > 0 ? [one] : [];
}

/**
 * Ensure `name` does not collide with existing provider names by appending a
 * numeric suffix (`-1`, `-2`, …). An empty name is returned unchanged (the user
 * fills it in the form).
 */
export function uniqueProviderName(
  name: string,
  existingNames: readonly string[],
): string {
  const base = name.trim();
  if (base === "") {
    return base;
  }
  const taken = new Set(existingNames);
  if (!taken.has(base)) {
    return base;
  }
  for (let i = 1; ; i += 1) {
    const candidate = `${base}-${i}`;
    if (!taken.has(candidate)) {
      return candidate;
    }
  }
}
