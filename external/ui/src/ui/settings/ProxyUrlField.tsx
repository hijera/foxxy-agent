import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type ClipboardEvent,
  type Ref,
} from "react";

import { useT } from "../i18n/I18nProvider";
import { ProxyEditorDialog } from "./ProxyEditorDialog";
import { applyMaskedEdit, maskProxyValue } from "./proxyUrl";

/** How long the character just typed into the password stays readable. */
export const PROXY_REVEAL_MS = 3000;

export type ProxyUrlFieldProps = {
  value: string;
  onChange: (value: string) => void;
  ariaLabel: string;
  placeholder?: string | undefined;
  title?: string | undefined;
  disabled?: boolean | undefined;
  /** Classes of the surface the field sits on (Settings or the onboarding dialog). */
  inputClassName: string;
  rowClassName: string;
  buttonClassName: string;
  dataTestId?: string | undefined;
  inputRef?: Ref<HTMLInputElement> | undefined;
};

/**
 * A proxy URL input that never shows the password: every character between
 * "user:" and the "@" is a dot, whether the field is being typed in or only
 * displayed, and the character just typed stays readable for three seconds the
 * way a phone's password field does. The "…" button opens ProxyEditorDialog,
 * where the URL is put together from separate fields.
 *
 * The masked text is exactly as long as the real one, so the field edits the
 * masked text as usual and applyMaskedEdit carries each edit over to the value.
 * Copying copies the real characters.
 */
export function ProxyUrlField(props: ProxyUrlFieldProps) {
  const {
    value,
    onChange,
    ariaLabel,
    placeholder,
    title,
    disabled,
    inputClassName,
    rowClassName,
    buttonClassName,
    dataTestId,
    inputRef,
  } = props;
  const { t } = useT();
  const [revealAt, setRevealAt] = useState<number | null>(null);
  const [editing, setEditing] = useState(false);
  const selection = useRef<[number, number] | undefined>(undefined);
  const caret = useRef<number | null>(null);
  const timer = useRef<number | undefined>(undefined);
  const ownRef = useRef<HTMLInputElement | null>(null);

  const shown = maskProxyValue(value, revealAt);

  const stopReveal = () => {
    window.clearTimeout(timer.current);
    timer.current = undefined;
    setRevealAt(null);
  };

  useEffect(() => () => window.clearTimeout(timer.current), []);

  // The browser moves the caret to the end whenever the value it shows is
  // replaced; put it back where the edit left it.
  useLayoutEffect(() => {
    const el = ownRef.current;
    const c = caret.current;
    if (el && c !== null && document.activeElement === el) {
      el.setSelectionRange(c, c);
      selection.current = [c, c];
    }
    caret.current = null;
  }, [shown]);

  const setRefs = (el: HTMLInputElement | null) => {
    ownRef.current = el;
    if (typeof inputRef === "function") {
      inputRef(el);
    } else if (inputRef && typeof inputRef === "object") {
      (inputRef as { current: HTMLInputElement | null }).current = el;
    }
  };

  const remember = (el: HTMLInputElement) => {
    selection.current = [el.selectionStart ?? 0, el.selectionEnd ?? 0];
  };

  const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
    const el = e.target;
    const edit = applyMaskedEdit(
      value,
      shown,
      el.value,
      selection.current,
      el.selectionStart ?? undefined,
    );
    window.clearTimeout(timer.current);
    if (edit.typedAt !== null) {
      setRevealAt(edit.typedAt);
      timer.current = window.setTimeout(() => {
        timer.current = undefined;
        setRevealAt(null);
      }, PROXY_REVEAL_MS);
    } else {
      setRevealAt(null);
    }
    caret.current = edit.caret;
    selection.current = [edit.caret, edit.caret];
    onChange(edit.value);
  };

  const handleCopy = (e: ClipboardEvent<HTMLInputElement>, cut: boolean) => {
    const el = e.currentTarget;
    const a = el.selectionStart ?? 0;
    const b = el.selectionEnd ?? 0;
    if (a === b) {
      return;
    }
    e.preventDefault();
    e.clipboardData.setData("text/plain", value.slice(a, b));
    if (cut) {
      stopReveal();
      caret.current = a;
      selection.current = [a, a];
      onChange(value.slice(0, a) + value.slice(b));
    }
  };

  return (
    <div className={rowClassName}>
      <input
        ref={setRefs}
        className={inputClassName}
        type="text"
        value={shown}
        placeholder={placeholder}
        title={title}
        aria-label={ariaLabel}
        disabled={disabled}
        autoComplete="off"
        autoCorrect="off"
        autoCapitalize="off"
        spellCheck={false}
        data-lpignore="true"
        data-testid={dataTestId}
        onChange={handleChange}
        onSelect={(e) => remember(e.currentTarget)}
        onKeyDown={(e) => remember(e.currentTarget)}
        onMouseUp={(e) => remember(e.currentTarget)}
        onBlur={stopReveal}
        onCopy={(e) => handleCopy(e, false)}
        onCut={(e) => handleCopy(e, true)}
      />
      <button
        type="button"
        className={buttonClassName}
        onClick={() => setEditing(true)}
        disabled={disabled}
        aria-label={t("settings.proxy.edit")}
        title={t("settings.proxy.edit")}
        data-testid={dataTestId ? `${dataTestId}-edit` : undefined}
      >
        …
      </button>
      {editing ? (
        <ProxyEditorDialog
          value={value}
          onCancel={() => setEditing(false)}
          onApply={(v) => {
            setEditing(false);
            stopReveal();
            onChange(v);
          }}
        />
      ) : null}
    </div>
  );
}
