import { useEffect, useRef, useState } from "react";
import { usePrefs } from "../app/prefs";
import { CheckIcon, CopyIcon } from "./Icons";

export function CopyField({ value, label }: { value: string; label: string }) {
  const { m } = usePrefs();
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      // No clipboard permission: select the text so the visitor can copy it by hand.
      const range = document.createRange();
      const node = document.getElementById(`copy-${label}`);
      if (!node) return;
      range.selectNodeContents(node);
      window.getSelection()?.removeAllRanges();
      window.getSelection()?.addRange(range);
      return;
    }
    setCopied(true);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setCopied(false), 1800);
  };

  return (
    <div className="copy-field">
      <span className="copy-label">{label}</span>
      <div className="copy-row">
        <code id={`copy-${label}`}>{value}</code>
        <button type="button" className="copy-btn" onClick={() => void copy()} aria-live="polite">
          {copied ? <CheckIcon /> : <CopyIcon />}
          <span>{copied ? m.install.copied : m.install.copy}</span>
        </button>
      </div>
    </div>
  );
}
