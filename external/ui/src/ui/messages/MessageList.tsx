import type { TranscriptItem } from '../chat/types';
import { AssistantMessage } from './AssistantMessage';
import { ToolCallMessage } from './ToolCallMessage';
import { UserMessage } from './UserMessage';

export function MessageList(props: { items: TranscriptItem[]; onLoadToolCallDetails?: (toolCallId: string) => void }) {
  return (
    <>
      {props.items.map((it) => {
        if (it.type === 'user_message') {
          return <UserMessage key={it.id} content={it.content} />;
        }
        if (it.type === 'assistant_message') {
          return <AssistantMessage key={it.id} content={it.content} />;
        }
        const toolProps: Record<string, unknown> = {
          toolCallId: it.toolCallId,
          status: it.status,
        };
        if (it.title !== undefined) toolProps.title = it.title;
        if (it.kind !== undefined) toolProps.kind = it.kind;
        if (it.argsText !== undefined) toolProps.argsText = it.argsText;
        if (it.resultText !== undefined) toolProps.resultText = it.resultText;
        if (it.detailsLoaded !== undefined) toolProps.detailsLoaded = it.detailsLoaded;
        return (
          <ToolCallMessage
            key={it.id}
            {...toolProps}
            {...(props.onLoadToolCallDetails ? { onLoadDetails: props.onLoadToolCallDetails } : {})}
          />
        );
      })}
    </>
  );
}

