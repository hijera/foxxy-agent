// The loop guard (internal/agent/react.go, loopAbortError and the repeated
// tool-call branch) ends a runaway turn with an English error, which the session
// manager records as a UI log entry. The text doubles as the `error` string of
// the HTTP/ACP turn, so the backend keeps it in English; localization happens
// here, at render time, the same way chat/compactionSummary.ts matches backend
// prefixes. Keep these literals byte-identical to the Go side.

const STREAM_NOTICE =
  "stopped: the model kept repeating the same output instead of finishing the task";
const REASONING_NOTICE =
  "stopped: the model kept repeating the same reasoning without reaching an answer";
// The tool notice interpolates the tool name, so only its prefix is stable:
// "stopped: the model kept requesting the same <tool> call with identical arguments".
const TOOL_NOTICE_PREFIX = "stopped: the model kept requesting the same ";
const TOOL_NOTICE_SUFFIX = " call with identical arguments";

/**
 * Returns the localized loop-guard notice for a backend error message, or null
 * when the message is not one of them (every other error renders verbatim).
 */
export function localizeLoopGuardNotice(
  message: string,
  t: (key: string, params?: Record<string, string | number>) => string,
): string | null {
  const text = (message ?? "").trim();
  if (text === STREAM_NOTICE) {
    return t("messages.loopGuardStream");
  }
  if (text === REASONING_NOTICE) {
    return t("messages.loopGuardReasoning");
  }
  if (text.startsWith(TOOL_NOTICE_PREFIX) && text.endsWith(TOOL_NOTICE_SUFFIX)) {
    const tool = text.slice(
      TOOL_NOTICE_PREFIX.length,
      text.length - TOOL_NOTICE_SUFFIX.length,
    );
    if (tool.length > 0) {
      return t("messages.loopGuardTool", { tool });
    }
  }
  return null;
}
