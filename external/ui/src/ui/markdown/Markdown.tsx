import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import { useCallback, useMemo, useState } from 'react';

type CodeProps = {
  inline?: boolean;
  className?: string | undefined;
  children?: unknown;
};

function normalizeText(children: unknown): string {
  if (Array.isArray(children)) {
    return children.map((c) => normalizeText(c)).join('');
  }
  if (typeof children === 'string') {
    return children;
  }
  return '';
}

function CopyButton(props: { text: string }) {
  const [copied, setCopied] = useState(false);

  const onCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(props.text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 900);
    } catch {
      try {
        const ta = document.createElement('textarea');
        ta.value = props.text;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.focus();
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 900);
      } catch {
        setCopied(false);
      }
    }
  }, [props.text]);

  return (
    <button type="button" className="md-copy" onClick={() => void onCopy()} aria-label="Copy code">
      {copied ? 'Copied' : 'Copy'}
    </button>
  );
}

export function Markdown(props: { text: string }) {
  const components = useMemo(
    () => ({
      code: (p: CodeProps) => {
        const txt = normalizeText(p.children);
        if (p.inline) {
          return <code className={p.className || ''}>{txt}</code>;
        }
        const isFenced = typeof p.className === 'string' && (p.className.includes('language-') || p.className.includes('hljs'));
        return (
          <div className="md-code">
            {isFenced ? <CopyButton text={txt.replace(/\n$/, '')} /> : null}
            <pre>
              <code className={p.className || ''}>{txt}</code>
            </pre>
          </div>
        );
      },
    }),
    [],
  );

  return (
    <div className="md">
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={components}>
        {props.text}
      </ReactMarkdown>
    </div>
  );
}
