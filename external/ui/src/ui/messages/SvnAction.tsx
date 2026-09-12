import { useT } from "../i18n/I18nProvider";
import { BrowserContent } from "./BrowserContent";
import {
  svnArgs,
  svnDiffTone,
  svnFailed,
  svnOperation,
  svnStatusLine,
} from "./svnActionDisplay";

export function SvnIcon() {
  return (
    <svg
      className="svn-tool-icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      aria-hidden="true"
    >
      <circle cx="6" cy="5" r="2.5" />
      <circle cx="6" cy="19" r="2.5" />
      <circle cx="18" cy="6" r="2.5" />
      <path d="M6 7.5v9M18 8.5v1a5 5 0 0 1-5 5H6" />
    </svg>
  );
}

const fieldKeys: Record<string, string> = {
  paths: "paths",
  target: "target",
  branch: "branch",
  source: "source",
  destination: "destination",
  revision: "revision",
  limit: "limit",
  recursive: "recursive",
  accept: "accept",
  message: "message",
  url: "url",
  "repository root": "repositoryRoot",
  "working copy root": "workingCopyRoot",
  note: "note",
  branches: "branches",
};
const statusKeys: Record<string, string> = {
  M: "modified",
  A: "added",
  D: "deleted",
  C: "conflicted",
  "?": "unversioned",
  "!": "missing",
  R: "replaced",
  I: "ignored",
  X: "external",
  "~": "obstructed",
};

export function SvnAction(props: {
  name: string;
  argsText?: string | undefined;
  resultText: string;
  status: string;
  permissionWaiting?: boolean | undefined;
  truncated?: boolean | undefined;
}) {
  const { t } = useT();
  const operation = svnOperation(props.name);
  const args = svnArgs(props.argsText);
  const result = props.resultText;
  const failed = svnFailed(props.status, result);
  const state = failed
    ? "failed"
    : props.status === "cancelled"
      ? "cancelled"
      : props.permissionWaiting
        ? "waiting"
        : props.status === "completed"
          ? "completed"
          : "running";
  const fields = args
    ? Object.entries(args).filter(
        ([, value]) =>
          value !== "" &&
          value !== null &&
          value !== undefined &&
          (!Array.isArray(value) || value.length > 0),
      )
    : [];
  const hasScope = fields.some(([key]) =>
    ["paths", "target", "branch", "source"].includes(key),
  );
  const fieldLabel = (key: string) =>
    fieldKeys[key] ? t(`messages.svn.${fieldKeys[key]}`) : key;
  const displayValue = (value: unknown) =>
    typeof value === "boolean"
      ? t(value ? "messages.svn.yes" : "messages.svn.no")
      : typeof value === "string" || typeof value === "number"
        ? String(value)
        : JSON.stringify(value);
  const structuredInfo =
    operation === "info" && !failed && /^branch: /m.test(result);
  const statusRows = operation === "status" && !failed;
  const diff = operation === "diff" && !failed;
  return (
    <div
      className={`svn-action svn-action--${state}`}
      aria-label={t("messages.svn.card")}
    >
      <div className="svn-action-head">
        <code className="svn-command">{props.name}</code>
        <span className={`svn-state svn-state--${state}`}>
          {t(`messages.svn.${state}`)}
        </span>
      </div>
      {args ? (
        <div className="svn-arguments">
          {!hasScope &&
            [
              "status",
              "diff",
              "log",
              "list",
              "commit",
              "update",
              "info",
            ].includes(operation || "") && (
              <div className="svn-scope">{t("messages.svn.wholeCopy")}</div>
            )}
          {fields.map(([key, value]) => (
            <div
              className={`svn-field${key === "message" ? " svn-field--message" : ""}`}
              key={key}
            >
              <span className="svn-field-label">{fieldLabel(key)}</span>
              <div className="svn-field-value">
                {Array.isArray(value) ? (
                  value.map((item, index) => (
                    <code className="svn-path" key={index}>
                      {displayValue(item)}
                    </code>
                  ))
                ) : (
                  <span>{displayValue(value)}</span>
                )}
              </div>
            </div>
          ))}
          {operation === "resolve" && !args.accept && (
            <div className="svn-field">
              <span className="svn-field-label">{fieldLabel("accept")}</span>
              <code>working</code>
            </div>
          )}
        </div>
      ) : (
        <BrowserContent
          label={t("messages.svn.arguments")}
          text={props.argsText || ""}
        >
          <pre>{props.argsText}</pre>
        </BrowserContent>
      )}
      {result && (
        <BrowserContent
          className="svn-result"
          label={t(failed ? "messages.svn.errorOutput" : "messages.svn.output")}
          text={result}
        >
          {structuredInfo ? (
            <dl className="svn-info">
              {result.split(/\r?\n/).map((line, index) => {
                const split = line.indexOf(": ");
                return split < 0 ? (
                  <div key={index}>{line}</div>
                ) : (
                  <div className="svn-field" key={index}>
                    <dt className="svn-field-label">
                      {fieldLabel(line.slice(0, split))}
                    </dt>
                    <dd className="svn-field-value">{line.slice(split + 2)}</dd>
                  </div>
                );
              })}
            </dl>
          ) : statusRows ? (
            <div className="svn-status-list">
              {result.split(/\r?\n/).map((line, index) => {
                const row = svnStatusLine(line);
                return row ? (
                  <div
                    className={`svn-status-row${row.conflict ? " svn-status-row--conflict" : ""}`}
                    key={index}
                  >
                    <code className="svn-status-code" title={row.columns}>
                      {row.columns}
                    </code>
                    <code className="svn-status-path">{row.path}</code>
                    <span className="svn-status-label">
                      {statusKeys[row.code]
                        ? t(`messages.svn.${statusKeys[row.code]}`)
                        : row.code}
                    </span>
                  </div>
                ) : (
                  <pre className="svn-output-line" key={index}>
                    {line === "working copy is clean"
                      ? t("messages.svn.clean")
                      : line}
                  </pre>
                );
              })}
            </div>
          ) : diff ? (
            <pre className="svn-diff">
              {result.split("\n").map((line, index, lines) => (
                <span
                  key={index}
                  className={`svn-diff-line svn-diff-line--${svnDiffTone(line)}`}
                >
                  {line}
                  {index < lines.length - 1 ? "\n" : ""}
                </span>
              ))}
            </pre>
          ) : (
            <pre>{result}</pre>
          )}
        </BrowserContent>
      )}
      {props.truncated && (
        <div className="svn-truncated">{t("messages.svn.truncated")}</div>
      )}
    </div>
  );
}
