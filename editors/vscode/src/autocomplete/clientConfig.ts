/** The autocomplete settings the backend hands to editor clients
 *  (GET /foxxycode/completion/config). They live in config.autocomplete, edited in the
 *  Autocomplete tab of the FoxxyCode settings form, so the extension never keeps a second
 *  copy of these knobs and contributes no settings of its own.
 *
 *  Mirrors `AutocompleteClientConfig.kt` in the IntelliJ plugin. */
export interface AutocompleteClientConfig {
  enabled: boolean;
  trigger: "auto" | "manual";
  debounceMs: number;
  multiLine: boolean;
  timeoutMs: number;
  maxPrefixBytes: number;
  maxSuffixBytes: number;
}

/** Safe state until the backend answers: an unreachable server must not start firing requests. */
export const DISABLED_CONFIG: AutocompleteClientConfig = {
  enabled: false,
  trigger: "auto",
  debounceMs: 350,
  multiLine: true,
  timeoutMs: 4000,
  maxPrefixBytes: 4000,
  maxSuffixBytes: 1500,
};

function bool(v: unknown, fallback: boolean): boolean {
  return typeof v === "boolean" ? v : fallback;
}

function positiveInt(v: unknown, fallback: number): number {
  return typeof v === "number" && Number.isFinite(v) && v > 0 ? Math.floor(v) : fallback;
}

/** Parses the endpoint's JSON body, falling back per field so a partial answer still works. */
export function parseClientConfig(json: string): AutocompleteClientConfig | null {
  let raw: unknown;
  try {
    raw = JSON.parse(json);
  } catch {
    return null;
  }
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null;
  const o = raw as Record<string, unknown>;
  return {
    enabled: bool(o["enabled"], DISABLED_CONFIG.enabled),
    trigger: o["trigger"] === "manual" ? "manual" : "auto",
    debounceMs: positiveInt(o["debounce_ms"], DISABLED_CONFIG.debounceMs),
    multiLine: bool(o["multi_line"], DISABLED_CONFIG.multiLine),
    timeoutMs: positiveInt(o["timeout_ms"], DISABLED_CONFIG.timeoutMs),
    maxPrefixBytes: positiveInt(o["max_prefix_bytes"], DISABLED_CONFIG.maxPrefixBytes),
    maxSuffixBytes: positiveInt(o["max_suffix_bytes"], DISABLED_CONFIG.maxSuffixBytes),
  };
}

/** True when suggestions should be requested while the user types, not only on the shortcut. */
export function suggestsWhileTyping(cfg: AutocompleteClientConfig): boolean {
  return cfg.enabled && cfg.trigger !== "manual";
}
