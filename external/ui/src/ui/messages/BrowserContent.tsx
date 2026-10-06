import {
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import hljs from "highlight.js/lib/core";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import css from "highlight.js/lib/languages/css";
import { useT } from "../i18n/I18nProvider";
import { CodeBlockCopyButton } from "./CodeBlockCopyButton";

hljs.registerLanguage("javascript", javascript);
hljs.registerLanguage("json", json);
hljs.registerLanguage("css", css);

export function BrowserHighlight({
  text,
  language,
}: {
  text: string;
  language: "javascript" | "json" | "css";
}) {
  const html = useMemo(
    () => hljs.highlight(text, { language, ignoreIllegals: true }).value,
    [text, language],
  );
  // highlight.js escapes source text before emitting its own span markup.
  return (
    <code
      className={`hljs language-${language}`}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

/** Keep a bounded viewport; disclose controls only after measuring actual overflow. */
export function BrowserContent({
  children,
  text,
  label,
  className = "",
}: {
  children: ReactNode;
  text: string;
  label: string;
  className?: string;
}) {
  const { t } = useT();
  const ref = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [overflow, setOverflow] = useState(false);
  useLayoutEffect(() => {
    if (expanded) return;
    const node = ref.current;
    if (!node) return;
    const measure = () =>
      setOverflow(node.scrollHeight > node.clientHeight + 1);
    measure();
    const details = node.closest("details");
    details?.addEventListener("toggle", measure);
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(measure);
    observer?.observe(node);
    window.addEventListener("resize", measure);
    return () => {
      observer?.disconnect();
      details?.removeEventListener("toggle", measure);
      window.removeEventListener("resize", measure);
    };
  }, [text, expanded]);
  return (
    <section className={`browser-content ${className}`}>
      <div className="browser-content-head">
        <span>{label}</span>
        <CodeBlockCopyButton textToCopy={text} />
      </div>
      <div
        ref={ref}
        className={`browser-content-viewport browser-content-viewport--${expanded ? "scroll" : "clip"}`}
        tabIndex={expanded ? 0 : undefined}
        role="region"
        aria-label={label}
      >
        {children}
      </div>
      {overflow && (
        <button
          type="button"
          className="tool-overflow-toggle"
          aria-expanded={expanded}
          onClick={() => {
            if (ref.current) ref.current.scrollTop = 0;
            setExpanded(!expanded);
          }}
        >
          {expanded ? t("messages.toolLess") : t("messages.toolMore")}
        </button>
      )}
    </section>
  );
}

export function BrowserCode({ expression }: { expression: string }) {
  const { t } = useT();
  const [formatted, setFormatted] = useState({ source: "", code: "" });
  useEffect(() => {
    let active = true;
    // Only parse the preview. Never execute the expression.
    void Promise.all([
      import("prettier/standalone"),
      import("prettier/plugins/babel"),
      import("prettier/plugins/estree"),
    ])
      .then(async ([prettier, babel, estree]) => {
        const code = await prettier.format(expression, {
          parser: "babel",
          plugins: [babel.default, estree.default],
          printWidth: 80,
        });
        if (active) setFormatted({ source: expression, code: code.trimEnd() });
      })
      .catch(() => {
        if (active) setFormatted({ source: expression, code: expression });
      });
    return () => {
      active = false;
    };
  }, [expression]);
  const text = formatted.source === expression ? formatted.code : expression;
  return (
    <BrowserContent
      className="browser-code"
      label={t("messages.browser.javascript")}
      text={text}
    >
      <pre>
        <BrowserHighlight text={text} language="javascript" />
      </pre>
    </BrowserContent>
  );
}
