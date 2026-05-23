import { useCallback, useState, startTransition } from "react";

import type {
  CoddyPermissionPayload,
  PermissionResolvedState,
} from "./permissionTypes";
import { permissionBodyText } from "./permissionTypes";
import { questionPromptFocusComposer } from "./QuestionPromptSection";

const HDR = "X-Coddy-Session-ID";

export type PermissionPromptSectionProps = {
  itemId: string;
  payload: CoddyPermissionPayload;
  resolved?: PermissionResolvedState | undefined;
  onResolved: (resolution: PermissionResolvedState) => void;
};

/** Inline permission gate for streaming permission SSE + POST /coddy/sessions/{id}/permission. */
export function PermissionPromptSection(props: PermissionPromptSectionProps) {
  const { payload, resolved, onResolved } = props;
  const [submitting, setSubmitting] = useState(false);
  const body = permissionBodyText(payload);
  const title =
    (payload.toolCall.title || "").trim() ||
    `Run: ${(payload.toolCall.kind || "tool").trim()}`;

  const choose = useCallback(
    async (optionId: string, label: string) => {
      const sid = payload.sessionId.trim();
      const tcid = payload.toolCall.toolCallId.trim();
      setSubmitting(true);
      try {
        try {
          await fetch(
            `/coddy/sessions/${encodeURIComponent(sid)}/permission`,
            {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
                [HDR]: sid,
              },
              body: JSON.stringify({ toolCallId: tcid, optionId }),
            },
          );
        } catch {
          // still unblock transcript on transient network errors
        }
        startTransition(() => {
          onResolved({ optionId, summaryLine: label });
        });
      } finally {
        setSubmitting(false);
      }
      questionPromptFocusComposer();
    },
    [onResolved, payload],
  );

  if (resolved) {
    return (
      <div className="permission-prompt-frame">
        <div className="permission-prompt-card permission-prompt-card--resolved">
          <div className="permission-prompt-head">
            <span className="permission-prompt-title">{title}</span>
            <span className="permission-prompt-resolved-badge">
              {resolved.summaryLine}
            </span>
          </div>
          {body ? (
            <pre className="permission-prompt-body">{body}</pre>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <div className="permission-prompt-frame">
      <div className="permission-prompt-card">
        <div className="permission-prompt-head">
          <span className="permission-prompt-icon" aria-hidden />
          <span className="permission-prompt-title">{title}</span>
        </div>
        {body ? <pre className="permission-prompt-body">{body}</pre> : null}
        <div className="permission-prompt-actions">
          {payload.options.map((opt) => {
            const isReject = opt.optionId === "reject";
            return (
              <button
                key={opt.optionId}
                type="button"
                className={
                  isReject
                    ? "permission-prompt-btn permission-prompt-btn--reject"
                    : "permission-prompt-btn permission-prompt-btn--allow"
                }
                disabled={submitting}
                onClick={() => void choose(opt.optionId, opt.name)}
              >
                {opt.name}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
