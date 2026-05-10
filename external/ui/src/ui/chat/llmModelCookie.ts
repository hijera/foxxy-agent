export const CODDY_LLM_MODEL_COOKIE = "coddy_llm_model";

const MAX_AGE_SECONDS = 365 * 24 * 60 * 60;

/** Reads last chosen YAML **`models[].model`** id for the embedded UI. */
export function readLlmModelCookie(): string | null {
  if (typeof document === "undefined") {
    return null;
  }
  const parts = document.cookie.split(";");
  for (const p of parts) {
    const s = p.trim();
    if (!s.startsWith(`${CODDY_LLM_MODEL_COOKIE}=`)) {
      continue;
    }
    const v = decodeURIComponent(
      s.slice(CODDY_LLM_MODEL_COOKIE.length + 1).trim(),
    );
    const t = v.trim();
    return t.length > 0 ? t : null;
  }
  return null;
}

export function writeLlmModelCookie(modelId: string): void {
  if (typeof document === "undefined") {
    return;
  }
  const v = modelId.trim();
  if (!v) {
    return;
  }
  const secure =
    typeof window !== "undefined" && window.location.protocol === "https:"
      ? "; Secure"
      : "";
  document.cookie = `${CODDY_LLM_MODEL_COOKIE}=${encodeURIComponent(v)}; Path=/; Max-Age=${MAX_AGE_SECONDS}; SameSite=Lax${secure}`;
}
