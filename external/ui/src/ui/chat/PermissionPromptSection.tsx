import { useCallback, useMemo, useState, startTransition } from "react";

import { PermissionToolPreview } from "./PermissionPromptPreview";
import {
  buildPermissionToolPreview,
  type PermissionToolCallContext,
} from "./permissionToolPreview";
import type {
  FoxxyCodePermissionPayload,
  PermissionResolvedState,
} from "./permissionTypes";
import { questionPromptFocusComposer } from "./QuestionPromptSection";
import { permissionOptionLabel } from "./permissionOptionLabel";
import { submitPermissionChoice } from "./permissionSubmit";
import { useT } from "../i18n/I18nProvider";

export type PermissionPromptSectionProps = {
  itemId: string;
  payload: FoxxyCodePermissionPayload;
  /** Matching transcript tool_call, whose argsText carries the structured arguments. */
  toolCall?: PermissionToolCallContext | undefined;
  resolved?: PermissionResolvedState | undefined;
  onResolved: (resolution: PermissionResolvedState) => void;
};

/** Inline permission gate for streaming permission SSE + POST /foxxycode/sessions/{id}/permission. */
export function PermissionPromptSection(props: PermissionPromptSectionProps) {
  const { payload, resolved, onResolved, toolCall } = props;
  const [submitting, setSubmitting] = useState(false);
  const preview = useMemo(
    () => buildPermissionToolPreview(payload, toolCall),
    [payload, toolCall],
  );

  const choose = useCallback(
    async (optionId: string, label: string) => {
      setSubmitting(true);
      try {
        try {
          await submitPermissionChoice(
            payload.sessionId,
            payload.toolCall.toolCallId,
            optionId,
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

  // Placed after the hooks so the component keeps a stable hook count between renders.
  if (resolved) {
    return null;
  }

  return (
    <div
      className="permission-prompt-frame"
      data-testid="permission-prompt-card"
    >
      <div className="permission-prompt-card">
        <div className="permission-prompt-head">
          <span
            className="permission-prompt-icon permission-prompt-icon--question"
            aria-hidden
          >
            ?
          </span>
          <span className="permission-prompt-title">{preview.title}</span>
          <code className="permission-prompt-tool-badge">
            {preview.toolName}
          </code>
        </div>
        <PermissionToolPreview preview={preview} />
        <div className="permission-prompt-actions">
          {payload.options.map((opt) => {
            const isReject = opt.optionId === "reject";
            const label = permissionOptionLabel(opt);
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
                onClick={() => void choose(opt.optionId, label)}
              >
                {label}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
