import { Markdown } from '../markdown/Markdown';

export function AssistantMessage(props: { content: string }) {
  return (
    <div className="msg msg-assistant">
      <Markdown text={props.content} />
    </div>
  );
}
