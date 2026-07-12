import { type ReactElement, useState } from "react";

import { useT } from "../i18n/I18nProvider";
import {
  type BrowserActionInfo,
  sessionAssetUrl,
} from "./browserActionDisplay";

/**
 * BrowserAction renders the result of an interactive browser tool call: a short
 * action summary, the resolved URL, the captured screenshot (click to enlarge),
 * and any console output. The screenshot is fetched from the session assets
 * endpoint by name.
 */
export function BrowserAction(props: {
  info: BrowserActionInfo;
  sessionId: string;
}): ReactElement {
  const { t } = useT();
  const [expanded, setExpanded] = useState(false);
  const { info, sessionId } = props;

  const shotUrl =
    info.screenshotName && sessionId
      ? sessionAssetUrl(sessionId, info.screenshotName)
      : "";

  return (
    <div className="browser-action" aria-label={t("messages.browserActionAriaLabel")}>
      {info.action ? (
        <div className="browser-action-line">{info.action}</div>
      ) : null}
      {info.url ? (
        <div className="browser-action-url muted" title={info.url}>
          {info.url}
        </div>
      ) : null}
      {shotUrl ? (
        <button
          type="button"
          className={
            expanded ? "browser-shot browser-shot--full" : "browser-shot"
          }
          onClick={() => setExpanded((v) => !v)}
          aria-label={
            expanded
              ? t("messages.browserShotCollapse")
              : t("messages.browserShotExpand")
          }
        >
          <img
            className="browser-shot-img"
            src={shotUrl}
            alt={t("messages.browserShotAlt")}
            loading="lazy"
          />
        </button>
      ) : null}
      {info.console.length > 0 ? (
        <details className="browser-console">
          <summary>{t("messages.browserConsole")}</summary>
          <pre className="tool-result-pre">{info.console.join("\n")}</pre>
        </details>
      ) : null}
    </div>
  );
}
