import { useMemo } from 'react';
import { Markdown } from '../markdown/Markdown';

function safePrettyJSON(text: string): string {
  try {
    const v = JSON.parse(text);
    return JSON.stringify(v, null, 2);
  } catch {
    return text;
  }
}

export function ToolCallMessage(props: {
  toolCallId: string;
  title?: string | undefined;
  kind?: string | undefined;
  status: string;
  argsText?: string | undefined;
  resultText?: string | undefined;
  detailsLoaded?: boolean;
  onLoadDetails?: (toolCallId: string) => void;
}) {
  const args = useMemo(() => (props.argsText ? safePrettyJSON(props.argsText) : ''), [props.argsText]);
  const result = useMemo(() => (props.resultText ? props.resultText : ''), [props.resultText]);
  const name = (props.title || props.kind || 'tool').trim();
  const showLoad = !!props.onLoadDetails && !props.detailsLoaded;

  return (
    <div className="msg msg-tools" data-kind={props.kind || ''} data-status={props.status}>
      <div className="tool-head">
        <span className="tool-name">{name}</span>
        <span className="tool-status">
          {showLoad ? (
            <button
              type="button"
              className="tool-more"
              onClick={() => props.onLoadDetails?.(props.toolCallId)}
              aria-label="Load tool details"
            >
              Details
            </button>
          ) : null}
          {props.status}
        </span>
      </div>
      {args ? (
        <pre className="tool-block" aria-label="Tool arguments">
          {args}
        </pre>
      ) : null}
      {result ? (
        <div className="tool-block tool-result" aria-label="Tool result">
          <Markdown text={result} />
        </div>
      ) : null}
    </div>
  );
}
