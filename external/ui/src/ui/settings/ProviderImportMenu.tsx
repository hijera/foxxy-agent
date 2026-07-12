import { useCallback, useEffect, useRef, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { parseProviderTransfer } from "./providerTransfer";
import { readClipboardText } from "./transferIO";

/**
 * Import control shown next to Add in the providers list: a toggle button that
 * reveals two sources — clipboard (a foxxycode://provider query string) or a
 * JSON file. Parsed providers are handed to onImport; secrets are stripped by
 * parseProviderTransfer. Returns the imported entries (empty array = nothing
 * usable, which we surface as an error).
 */
export function ProviderImportMenu(props: {
  onImport: (items: Record<string, string>[]) => void;
}) {
  const { t } = useT();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  // Close the popover on outside click / Escape.
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

  const apply = useCallback(
    (text: string) => {
      try {
        const items = parseProviderTransfer(text);
        if (items.length === 0) {
          setError(t("settings.providers.importFailed"));
          return;
        }
        setError(null);
        setOpen(false);
        props.onImport(items);
      } catch {
        setError(t("settings.providers.importFailed"));
      }
    },
    [props, t],
  );

  const fromClipboard = useCallback(async () => {
    const text = await readClipboardText();
    if (text === null) {
      setError(t("settings.providers.importFailed"));
      return;
    }
    apply(text);
  }, [apply, t]);

  const onFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      e.target.value = "";
      if (!file) {
        return;
      }
      const reader = new FileReader();
      reader.onload = () => apply(String(reader.result ?? ""));
      reader.onerror = () => setError(t("settings.providers.importFailed"));
      reader.readAsText(file);
    },
    [apply, t],
  );

  return (
    <div className="provider-import" ref={rootRef}>
      <button
        type="button"
        className="settings-btn"
        data-testid="provider-import-toggle"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => {
          setError(null);
          setOpen((v) => !v);
        }}
      >
        {t("settings.providers.import")}
      </button>
      {open ? (
        <div className="provider-import-menu" role="menu">
          <button
            type="button"
            role="menuitem"
            className="provider-import-item"
            data-testid="provider-import-clipboard"
            onClick={() => void fromClipboard()}
          >
            {t("settings.providers.importFromClipboard")}
          </button>
          <button
            type="button"
            role="menuitem"
            className="provider-import-item"
            data-testid="provider-import-file"
            onClick={() => fileRef.current?.click()}
          >
            {t("settings.providers.importFromFile")}
          </button>
        </div>
      ) : null}
      <input
        ref={fileRef}
        type="file"
        accept=".json,application/json"
        className="provider-import-file-input"
        data-testid="provider-import-file-input"
        onChange={onFileChange}
      />
      {error ? (
        <span className="provider-import-error" role="alert">
          {error}
        </span>
      ) : null}
    </div>
  );
}
