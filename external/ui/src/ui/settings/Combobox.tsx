import { type KeyboardEvent, useEffect, useMemo, useRef, useState } from "react";
import { useT } from "../i18n/I18nProvider";

export type ComboOption = { value: string; label?: string };

/**
 * Combobox is an editable select: a text input the user can type into freely,
 * with a filterable dropdown of suggestions. Focusing or clicking the caret shows
 * all options; typing filters them. Any typed value is kept (free text), so it
 * doubles as a plain input when no option matches. Used for every settings field
 * that used to be a plain <select> (schema enums, provider, model ids).
 */
export function Combobox(props: {
  value: string;
  onChange: (v: string) => void;
  options: ComboOption[];
  placeholder?: string | undefined;
  ariaLabel?: string | undefined;
  testid?: string | undefined;
  disabled?: boolean | undefined;
  /**
   * When true, the collapsed input shows the matching option's label instead of
   * the raw value (used for enum fields with translated labels). While focused the
   * raw value is shown so it stays editable. Off by default so free-text
   * comboboxes (provider, model id) keep showing what the user typed.
   */
  showOptionLabel?: boolean | undefined;
  /**
   * When true, the dropdown list opens upward (above the input) instead of below.
   * Used where the field sits near the bottom of a constrained container (e.g. the
   * onboarding modal) so the list stays visible instead of being clipped.
   */
  openUp?: boolean | undefined;
}) {
  const { t } = useT();
  const { value, onChange, options, placeholder, ariaLabel, testid, disabled } = props;
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState(false);
  const [focused, setFocused] = useState(false);
  const [highlight, setHighlight] = useState(-1);
  const rootRef = useRef<HTMLDivElement>(null);

  const shownValue =
    props.showOptionLabel && !focused
      ? (options.find((o) => o.value === value)?.label ?? value)
      : value;

  const filtered = useMemo(() => {
    if (!typed) {
      return options;
    }
    const q = value.trim().toLowerCase();
    if (!q) {
      return options;
    }
    return options.filter(
      (o) =>
        o.value.toLowerCase().includes(q) ||
        (o.label ? o.label.toLowerCase().includes(q) : false),
    );
  }, [options, value, typed]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onDoc = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  const pick = (o: ComboOption) => {
    onChange(o.value);
    setTyped(false);
    setOpen(false);
    setHighlight(-1);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (!open) {
        setOpen(true);
        setTyped(false);
        return;
      }
      setHighlight((h) => Math.min(h + 1, filtered.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setHighlight((h) => Math.max(h - 1, 0));
    } else if (e.key === "Enter") {
      if (open && highlight >= 0 && highlight < filtered.length) {
        e.preventDefault();
        pick(filtered[highlight]!);
      }
    } else if (e.key === "Escape") {
      setOpen(false);
    }
  };

  return (
    <div className="settings-combobox" ref={rootRef}>
      <input
        className="settings-input settings-combobox-input"
        type="text"
        role="combobox"
        aria-expanded={open}
        aria-autocomplete="list"
        aria-label={ariaLabel}
        data-testid={testid}
        value={shownValue}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(e) => {
          onChange(e.target.value);
          setTyped(true);
          setOpen(true);
          setHighlight(-1);
        }}
        onFocus={() => {
          setFocused(true);
          setTyped(false);
          setOpen(true);
        }}
        onBlur={() => setFocused(false)}
        onKeyDown={onKeyDown}
      />
      <button
        type="button"
        className="settings-combobox-toggle"
        tabIndex={-1}
        aria-label={t("settings.toggleOptions")}
        disabled={disabled}
        onMouseDown={(e) => {
          e.preventDefault();
          setTyped(false);
          setOpen((o) => !o);
        }}
      >
        ▾
      </button>
      {open && filtered.length > 0 ? (
        <ul
          className={`settings-combobox-list${props.openUp ? " settings-combobox-list--up" : ""}`}
          role="listbox"
        >
          {filtered.map((o, i) => (
            <li
              key={o.value}
              role="option"
              aria-selected={o.value === value}
              className={`settings-combobox-option${i === highlight ? " is-highlight" : ""}${
                o.value === value ? " is-current" : ""
              }`}
              onMouseDown={(e) => {
                e.preventDefault();
                pick(o);
              }}
              onMouseEnter={() => setHighlight(i)}
            >
              {o.label || o.value}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
