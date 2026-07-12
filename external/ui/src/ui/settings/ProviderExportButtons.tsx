import { useCallback, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  providerToClipboard,
  providerToJson,
  sanitizeProvider,
} from "./providerTransfer";
import { copyText, downloadTextFile } from "./transferIO";

/**
 * Export controls shown under a single provider's form: download the safe
 * fields as a JSON file, or copy them to the clipboard as a foxxycode://provider
 * query string. API keys and proxy are never included (see providerTransfer).
 */
export function ProviderExportButtons(props: {
  provider: Record<string, unknown>;
}) {
  const { t } = useT();
  const [status, setStatus] = useState<string | null>(null);

  const safeName =
    sanitizeProvider(props.provider).name?.replace(/[^a-zA-Z0-9_-]/g, "") || "provider";

  const flash = useCallback((msg: string) => {
    setStatus(msg);
    window.setTimeout(() => setStatus(null), 2200);
  }, []);

  const onFile = useCallback(() => {
    downloadTextFile(`provider-${safeName}.json`, providerToJson(props.provider));
    flash(t("settings.providers.exportedFile"));
  }, [props.provider, safeName, flash, t]);

  const onClipboard = useCallback(async () => {
    const ok = await copyText(providerToClipboard(props.provider));
    flash(ok ? t("settings.providers.exportCopied") : t("settings.providers.importFailed"));
  }, [props.provider, flash, t]);

  return (
    <div className="provider-export" data-testid="provider-export">
      <p className="settings-field-desc">{t("settings.providers.secretsExcludedHint")}</p>
      <div className="provider-export-actions">
        <button
          type="button"
          className="settings-btn"
          data-testid="provider-export-file"
          onClick={onFile}
        >
          {t("settings.providers.exportFile")}
        </button>
        <button
          type="button"
          className="settings-btn"
          data-testid="provider-export-clipboard"
          onClick={() => void onClipboard()}
        >
          {t("settings.providers.exportClipboard")}
        </button>
        {status ? (
          <span className="provider-export-status" role="status">
            {status}
          </span>
        ) : null}
      </div>
    </div>
  );
}
