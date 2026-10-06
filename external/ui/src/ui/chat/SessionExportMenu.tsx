import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n/I18nProvider";

/**
 * Export formats offered by the per-session download action. Mirrors the
 * `format` query parameter accepted by GET /foxxycode/sessions/{id}/export.
 */
export type ExportFormat = "pdf" | "docx" | "html" | "json";

const EXPORT_FORMATS: ExportFormat[] = ["pdf", "docx", "html", "json"];

/**
 * i18n key for each format label (e.g. chat.exportPdf). Precomputed so we avoid
 * runtime string indexing under noUncheckedIndexedAccess.
 */
const EXPORT_LABEL_KEYS: Record<ExportFormat, string> = {
  pdf: "chat.exportPdf",
  docx: "chat.exportDocx",
  html: "chat.exportHtml",
  json: "chat.exportJson",
};

/**
 * Per-session export menu rendered next to the chat title. Shows a download
 * glyph whose dropdown lists the four document formats. The caller gates the
 * whole control on `hasAssistant` (at least one assistant answer), so this
 * component assumes that is already true when it renders.
 *
 * The dropdown pattern (rootRef + outside-mousedown/Escape close) mirrors
 * ProviderImportMenu; the trigger is an icon button styled like the chat
 * title button.
 */
export function SessionExportMenu(props: {
  onExport: (format: ExportFormat) => void;
  busy?: boolean;
}) {
  const { t } = useT();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  // Close the popover on outside click / Escape — same lifecycle as the
  // provider import menu.
  useEffect(() => {
    if (!open) {
      return;
    }
    const onDoc = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const disabled = props.busy === true;

  return (
    <div className="session-export" ref={rootRef}>
      <button
        type="button"
        className="session-export-toggle"
        data-testid="session-export-toggle"
        aria-haspopup="menu"
        aria-expanded={open}
        disabled={disabled}
        title={t("chat.exportLabel")}
        aria-label={t("chat.exportLabel")}
        onClick={() => {
          if (!disabled) {
            setOpen((v) => !v);
          }
        }}
      >
        {disabled ? <IconSpinner /> : <IconDownload />}
      </button>
      {open ? (
        <div className="session-export-menu" role="menu">
          {EXPORT_FORMATS.map((fmt) => (
            <button
              key={fmt}
              type="button"
              role="menuitem"
              className="session-export-item"
              data-testid={`session-export-${fmt}`}
              disabled={disabled}
              onClick={() => {
                setOpen(false);
                props.onExport(fmt);
              }}
            >
              {t(EXPORT_LABEL_KEYS[fmt])}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function IconDownload() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.9}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M12 3v12" />
      <polyline points="7 10 12 15 17 10" />
      <path d="M5 21h14" />
    </svg>
  );
}

function IconSpinner() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      aria-hidden
    >
      <path d="M21 12a9 9 0 1 1-6.219-8.56" className="session-export-spin" />
    </svg>
  );
}
