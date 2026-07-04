import { t } from "../i18n/i18n";

/** Extract user-visible text from a parsed OpenAI-style SSE JSON line that carries `error`. */
export function openAIStreamErrorMessage(parsed: unknown): string | null {
  if (!parsed || typeof parsed !== "object") {
    return null;
  }
  const err = (parsed as { error?: unknown }).error;
  if (err === undefined || err === null) {
    return null;
  }
  if (typeof err === "string") {
    const m = err.trim();
    return m.length > 0 ? m : t("app.requestFailed");
  }
  if (typeof err === "object") {
    const msg = (err as { message?: unknown }).message;
    if (typeof msg === "string" && msg.trim() !== "") {
      return msg.trim();
    }
    return t("app.requestFailed");
  }
  return t("app.requestFailed");
}
