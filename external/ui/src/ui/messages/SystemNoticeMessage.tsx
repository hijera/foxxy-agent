import {
  formatUtcToLocalFullDetail,
  formatUtcToLocalHM,
} from "./formatMessageTime";
import { useT } from "../i18n/I18nProvider";
import { MessageCopyIconButton } from "./MessageCopyIconButton";

/** First non-empty line of the message, used as the collapsed preview. */
function firstLine(message: string): string {
  return (
    message
      .split(/\r?\n/)
      .map((line) => line.trim())
      .find((line) => line.length > 0) ?? ""
  );
}

export function SystemNoticeMessage(props: {
  level: "error";
  message: string;
  createdAtUtc?: string;
}) {
  const { t } = useT();
  const timeHM = props.createdAtUtc
    ? formatUtcToLocalHM(props.createdAtUtc)
    : "";
  const timeFull =
    props.createdAtUtc && timeHM
      ? formatUtcToLocalFullDetail(props.createdAtUtc)
      : "";
  const message = props.message ?? "";
  const preview = firstLine(message);
  return (
    <div className="msg-system-stack">
      <div className={`msg msg-system msg-system-${props.level}`} role="alert">
        {/* Collapsed by default: the full (often long) error body is behind a disclosure so it
            can be read on demand without flooding the chat. */}
        <details className="msg-system-details">
          <summary
            className="msg-system-summary"
            aria-label={t("messages.systemToggleAriaLabel")}
          >
            <span className="msg-system-chevron" aria-hidden="true" />
            <span className="msg-system-label">{t("messages.systemLabel")}</span>
            {preview ? (
              <span className="msg-system-preview">{preview}</span>
            ) : null}
          </summary>
          <pre className="msg-system-body">{message}</pre>
        </details>
      </div>
      <div className="msg-system-foot">
        <MessageCopyIconButton
          textToCopy={props.message}
          tooltip={t("messages.copyMessage")}
          ariaLabel={t("messages.copyErrorMessage")}
          dataTestId="system-message-copy"
        />
        {timeHM ? (
          <time
            className="msg-system-time"
            dateTime={props.createdAtUtc}
            title={timeFull || undefined}
          >
            {timeHM}
          </time>
        ) : null}
      </div>
    </div>
  );
}
