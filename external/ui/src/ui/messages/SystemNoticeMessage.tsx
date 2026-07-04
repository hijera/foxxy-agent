import {
  formatUtcToLocalFullDetail,
  formatUtcToLocalHM,
} from "./formatMessageTime";
import { useT } from "../i18n/I18nProvider";
import { MessageCopyIconButton } from "./MessageCopyIconButton";

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
  return (
    <div className="msg-system-stack">
      <div className={`msg msg-system msg-system-${props.level}`} role="alert">
        <div className="msg-system-label">{t("messages.systemLabel")}</div>
        <pre className="msg-system-body">{props.message}</pre>
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
