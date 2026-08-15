import { useEffect, useRef, useState, type ReactNode } from "react";
import { useT } from "../i18n/I18nProvider";

export function ChatHeader(props: {
  title: string;
  editable?: boolean;
  onTitleSave?: (title: string) => void;
  onCreateMiniApp?: () => void;
  /**
   * Per-session actions rendered at the right edge of the header row. They live
   * inside the header element so the flex row aligns them with the title
   * instead of pushing them onto a line of their own below the card.
   */
  actions?: ReactNode;
}) {
  const { t } = useT();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(props.title || "");
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!editing) {
      setValue(props.title || "");
    }
  }, [props.title, editing]);

  useEffect(() => {
    if (editing) {
      inputRef.current?.focus();
      inputRef.current?.select();
    }
  }, [editing]);

  const canEdit = !!props.editable && !!props.onTitleSave;

  return (
    <header className="chat-header">
      <div className="chat-title" id="chat-title">
        {canEdit && editing ? (
          <input
            ref={inputRef}
            className="chat-title-input"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onBlur={() => {
              setEditing(false);
              const t = value.trim();
              if (t && t !== (props.title || "").trim()) {
                props.onTitleSave?.(t);
              }
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                (e.target as HTMLInputElement).blur();
              }
              if (e.key === "Escape") {
                setValue(props.title || "");
                setEditing(false);
              }
            }}
          />
        ) : (
          <button
            type="button"
            className={`chat-title-btn ${canEdit ? "is-editable" : ""}`}
            onClick={() => {
              if (canEdit) {
                setEditing(true);
              }
            }}
            aria-label={t("chat.chatTitleAriaLabel")}
          >
            {props.title || t("chat.newChat")}
          </button>
        )}
      </div>
      {props.onCreateMiniApp ? (
        <button
          type="button"
          className="chat-miniapp-action"
          aria-label={t("miniapps.createFromSessionAria")}
          onClick={props.onCreateMiniApp}
        >
          <svg
            className="chat-miniapp-action-icon"
            viewBox="0 0 20 20"
            aria-hidden="true"
          >
            <rect x="2.5" y="2.5" width="5" height="5" rx="1" />
            <rect x="2.5" y="12.5" width="5" height="5" rx="1" />
            <rect x="12.5" y="2.5" width="5" height="5" rx="1" />
            <path d="M15 11.5v6M12 14.5h6" />
          </svg>
          <span className="chat-miniapp-action-label">
            {t("miniapps.createFromSession")}
          </span>
        </button>
      ) : null}
      {props.actions ?? null}
    </header>
  );
}
