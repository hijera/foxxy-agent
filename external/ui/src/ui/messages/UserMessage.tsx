import { Markdown } from "../markdown/Markdown";
import { slugSlashesForUserBubbleMarkdown } from "../skills/segmentComposerSlashSpans";
import { stripCoddyAttachmentsForUserDisplay } from "../skills/stripCoddyAttachments";

export function UserMessage(props: { content: string }) {
  const display = slugSlashesForUserBubbleMarkdown(
    stripCoddyAttachmentsForUserDisplay(props.content),
  );
  return (
    <div className="msg msg-user">
      <Markdown text={display} />
    </div>
  );
}
