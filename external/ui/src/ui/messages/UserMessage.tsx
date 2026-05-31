import { stripCoddyAttachmentsForUserDisplay } from "../skills/stripCoddyAttachments";
import {
  formatUtcToLocalFullDetail,
  formatUtcToLocalHM,
} from "./formatMessageTime";
import { MessageCopyIconButton } from "./MessageCopyIconButton";

export function UserMessage(props: {
  content: string;
  createdAtUtc?: string;
  /** Called when the user clicks the Edit button. */
  onEdit?: (content: string) => void;
}) {
  const display = stripCoddyAttachmentsForUserDisplay(props.content);
  const timeHM = props.createdAtUtc
    ? formatUtcToLocalHM(props.createdAtUtc)
    : "";
  const timeFull =
    props.createdAtUtc && timeHM
      ? formatUtcToLocalFullDetail(props.createdAtUtc)
      : "";
  return (
    <div className="msg-user-stack">
      <div className="msg msg-user msg-user--editable">
        <div className="msg-user-body" data-testid="user-message-body">
          {display}
        </div>
        {props.onEdit ? (
          <button
            type="button"
            className="msg-user-edit"
            aria-label="Edit message"
            title="Edit message"
            data-testid="user-message-edit"
            onClick={() => props.onEdit!(props.content)}
          >
            ✎
          </button>
        ) : null}
      </div>
      <div className="msg-user-foot">
        <MessageCopyIconButton
          textToCopy={props.content}
          tooltip="Copy message"
          ariaLabel="Copy message"
          dataTestId="user-message-copy"
        />
        {timeHM ? (
          <time
            className="msg-user-time"
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
