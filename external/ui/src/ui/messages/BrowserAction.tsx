import { type ReactElement, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  BrowserCode,
  BrowserContent,
  BrowserHighlight,
} from "./BrowserContent";
import {
  browserArgs,
  browserOperation,
  parseBrowserActionResult,
  scrollOffset,
  sessionAssetUrl,
  signedOffset,
} from "./browserActionDisplay";

export function BrowserIcon() {
  return (
    <svg
      className="browser-tool-icon"
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      aria-hidden="true"
    >
      <rect x="2" y="3" width="16" height="14" rx="3" />
      <path d="M2 7h16M5 5h1m2 0h1" />
    </svg>
  );
}

function ScrollDiagram({ x, y }: { x: number; y: number }) {
  const { t } = useT();
  const magnitude = Math.max(Math.abs(x), Math.abs(y), 1);
  const dx = (x / magnitude) * 48;
  const dy = (y / magnitude) * 48;
  const angle = (Math.atan2(dy, dx) * 180) / Math.PI;
  const label = t("messages.browser.offsetAria", {
    x: signedOffset(x),
    y: signedOffset(y),
  });
  return (
    <div className="browser-scroll-screen" role="img" aria-label={label}>
      <div className="browser-scroll-toolbar" aria-hidden="true">
        <span className="browser-scroll-window-dots">
          <i />
          <i />
          <i />
        </span>
        <span className="browser-scroll-address" />
      </div>
      <svg viewBox="0 0 260 174" aria-hidden="true">
        <path className="browser-scroll-guides" d="M24 74h212M130 16v116" />
        <circle cx="130" cy="74" r="3" fill="currentColor" />
        {(x !== 0 || y !== 0) && (
          <g
            className="browser-scroll-arrow"
            stroke="currentColor"
            strokeWidth="2.5"
            fill="none"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d={`M130 74 L${130 + dx} ${74 + dy}`} />
            <path
              d="M-9 -5 L0 0 L-9 5"
              transform={`translate(${130 + dx} ${74 + dy}) rotate(${angle})`}
            />
          </g>
        )}
        <text x="130" y="154" textAnchor="middle">
          {x === 0 && y === 0
            ? t("messages.browser.noOffset")
            : t("messages.browser.offset", {
                x: signedOffset(x),
                y: signedOffset(y),
              })}
        </text>
      </svg>
    </div>
  );
}

function PageLog({ text }: { text: string }) {
  return (
    <pre>
      {text.split("\n").map((line, i) => {
        const tone = /\b(error|exception|failed)\b|\b[45]\d\d\b/i.test(line)
          ? "error"
          : /\bwarn(ing)?\b/i.test(line)
            ? "warning"
            : "normal";
        return (
          <span
            className={`browser-log-line browser-log-line--${tone}`}
            key={i}
          >
            {line}
            {"\n"}
          </span>
        );
      })}
    </pre>
  );
}

function InspectReport({ text }: { text: string }) {
  const { t } = useT();
  return (
    <table className="browser-inspect-table">
      <thead>
        <tr>
          <th>{t("messages.browser.property")}</th>
          <th>{t("messages.browser.value")}</th>
        </tr>
      </thead>
      <tbody>
        {text
          .split("\n")
          .filter((line) => line.trim())
          .map((line, index) => {
            const match =
              line.match(/^\s*(.*?)\s+=\s+(.*)$/) ||
              line.match(/^\s*([^:]+):\s+(.+)$/);
            return match ? (
              <tr key={index}>
                <th scope="row">{match[1]}</th>
                <td>{match[2]}</td>
              </tr>
            ) : (
              <tr key={index}>
                <td colSpan={2}>{line}</td>
              </tr>
            );
          })}
      </tbody>
    </table>
  );
}

export function BrowserAction(props: {
  name: string;
  argsText?: string | undefined;
  resultText: string;
  status: string;
  sessionId: string;
}): ReactElement {
  const { t } = useT();
  const [expandedShot, setExpandedShot] = useState(false);
  const [failedShot, setFailedShot] = useState("");
  const operation = browserOperation(props.name);
  const args = browserArgs(props.argsText);
  const arg = (key: string) =>
    typeof args[key] === "string" ? (args[key] as string) : "";
  const info = parseBrowserActionResult(props.resultText);
  const shotUrl =
    info?.screenshotName && props.sessionId
      ? sessionAssetUrl(props.sessionId, info.screenshotName)
      : "";
  const failed =
    props.status === "failed" || /^error:/i.test(props.resultText.trim());
  const textResult = ["evaluate", "read_page", "page_log", "inspect"].includes(
    operation,
  );
  let result = textResult || failed ? props.resultText : info?.body || "";
  if (operation === "evaluate" && !failed) {
    const raw = result.replace(/^result:\s*/, "");
    try {
      result = JSON.stringify(JSON.parse(raw), null, 2);
    } catch {
      result = raw;
    }
  }
  const selector = arg("selector").trim();
  const hasArgs = Object.keys(args).length > 0;
  return (
    <div
      className={`browser-action${failed ? " browser-action--failed" : ""}`}
      aria-label={t("messages.browserActionAriaLabel")}
    >
      {operation === "navigate" && arg("url") && (
        <div className="browser-action-url">{arg("url")}</div>
      )}
      {selector && (
        <div className="browser-selector">
          <span className="browser-field-label">
            {t("messages.browser.selector")}
          </span>
          <BrowserHighlight text={selector} language="css" />
        </div>
      )}
      {operation === "evaluate" && arg("expression") && (
        <BrowserCode expression={arg("expression")} />
      )}
      {operation === "fill" && (
        <BrowserContent
          label={t("messages.browser.inputText")}
          text={arg("text")}
        >
          <pre>{arg("text")}</pre>
        </BrowserContent>
      )}
      {operation === "scroll" &&
        !selector &&
        (hasArgs || props.argsText?.trim() === "{}") && (
          <ScrollDiagram x={scrollOffset(args.x)} y={scrollOffset(args.y)} />
        )}
      {!hasArgs && props.argsText && props.argsText.trim() !== "{}" && (
        <BrowserContent
          label={t("messages.browser.arguments")}
          text={props.argsText}
        >
          <pre>{props.argsText}</pre>
        </BrowserContent>
      )}
      {info && (
        <div
          className={`browser-result${failed ? " browser-result--failed" : ""}`}
        >
          {!result && (
            <div className="tool-call-result-head">
              <span className="tool-call-result-dot" aria-hidden="true" />
              <span>
                {t(
                  failed
                    ? "messages.browser.error"
                    : "messages.toolResultSection",
                )}
              </span>
            </div>
          )}
          {info.url && !textResult && (
            <div className="browser-action-url muted">{info.url}</div>
          )}
          {shotUrl && failedShot !== shotUrl && (
            <button
              type="button"
              className={`browser-shot${expandedShot ? " browser-shot--full" : ""}`}
              onClick={() => setExpandedShot(!expandedShot)}
              aria-expanded={expandedShot}
              aria-label={t(
                expandedShot
                  ? "messages.browserShotCollapse"
                  : "messages.browserShotExpand",
              )}
            >
              <img
                className="browser-shot-img"
                src={shotUrl}
                alt={t("messages.browserShotAlt")}
                loading="lazy"
                onError={() => setFailedShot(shotUrl)}
              />
            </button>
          )}
          {shotUrl && failedShot === shotUrl && (
            <p className="muted">{t("messages.browser.imageUnavailable")}</p>
          )}
          {info.screenshotUnavailable && !failed && (
            <div
              className="browser-result-note muted"
              title={info.screenshotUnavailable}
            >
              {t(
                info.screenshotUnavailable.startsWith("disabled")
                  ? "messages.browser.screenshotsDisabled"
                  : "messages.browser.imageUnavailable",
              )}
            </div>
          )}
          {result && (
            <BrowserContent
              label={t(
                failed
                  ? "messages.browser.error"
                  : "messages.toolResultSection",
              )}
              text={result}
            >
              {operation === "evaluate" && !failed ? (
                <pre>
                  <BrowserHighlight text={result} language="json" />
                </pre>
              ) : operation === "inspect" && !failed ? (
                <InspectReport text={result} />
              ) : operation === "page_log" && !failed ? (
                <PageLog text={result} />
              ) : (
                <pre>{result}</pre>
              )}
            </BrowserContent>
          )}
          {!textResult && info.console.length > 0 && (
            <BrowserContent
              label={t("messages.browser.page_log")}
              text={info.console.join("\n")}
            >
              <PageLog text={info.console.join("\n")} />
            </BrowserContent>
          )}
          {!result &&
            !shotUrl &&
            !info.console.length &&
            !info.screenshotUnavailable && (
              <div className="browser-result-note">
                {operation === "close" && props.status === "completed"
                  ? t("messages.browser.closed")
                  : info.action}
              </div>
            )}
        </div>
      )}
    </div>
  );
}
