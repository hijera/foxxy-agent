import { Markdown } from '../markdown/Markdown';

export function UserMessage(props: { content: string }) {
  return (
    <div className="msg msg-user">
      <Markdown text={props.content} />
    </div>
  );
}
