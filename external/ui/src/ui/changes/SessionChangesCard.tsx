import { useEffect, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { baseName, dirName } from "./sessionChangesText";
import { useDiscard } from "./useDiscard";
import { hasEdits, type WorkingCopy } from "./workingCopy";

const ROW_CAP = 8;

type CardProps = {
  sessionId: string;
  workingCopy: WorkingCopy;
  onReview: () => void;
  onFile: (path: string) => void;
};

/** The shortcut stays active when the transcript window has detached its newest rows. */
export function useChangesCard(sessionId: string, enabled: boolean, workingCopy: WorkingCopy): boolean {
  const [choice, setChoice] = useState<{ sessionId: string; shown: boolean }>();
  const shown = choice?.sessionId === sessionId ? choice.shown : hasEdits(workingCopy);

  useEffect(() => {
    if (!enabled || !sessionId) return;
    const onKey = (event: KeyboardEvent) => {
      if (
        !(event.ctrlKey || event.metaKey) ||
        event.altKey || event.shiftKey || event.code !== "KeyS"
      ) return;
      event.preventDefault();
      if (!event.repeat) setChoice({ sessionId, shown: !shown });
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [enabled, sessionId, shown]);

  return enabled && shown;
}

export function SessionChangesCard(props: CardProps) {
  const { t } = useT();
  if (!hasEdits(props.workingCopy)) {
    return (
      <section className="changes-card">
        <p className="changes-card-empty" data-testid="changes-card-empty">
          {props.workingCopy.error || t("changes.empty")}
        </p>
      </section>
    );
  }

  return <ChangedFiles {...props} />;
}

function ChangedFiles(props: CardProps) {
  const { t, tp } = useT();
  const { changes } = props.workingCopy;
  const discard = useDiscard(props.sessionId);
  const { additions, deletions, files } = changes.totals;
  const hidden = changes.files.length - ROW_CAP;
  const skipped = changes.skipped ?? 0;
  return (
    <section className="changes-card" data-testid="session-changes-card">
      <div className="changes-card-head">
        <button type="button" className="changes-card-summary" data-testid="changes-card-open" onClick={() => props.onReview()}>
          <span className="changes-card-title">{tp("changes.card.fileCount", files)}</span>
          <span className="changes-card-stat">
            <span className="changes-add">{"+" + additions}</span>
            <span className="changes-del">{"−" + deletions}</span>
          </span>
        </button>
        <div className="changes-card-actions">
          <button type="button" className="changes-action" disabled={discard.busy} onClick={() => void discard.discardAll()}>
            {t("changes.discardAll")}
          </button>
          <button type="button" className="changes-action changes-action--primary" data-testid="changes-review" onClick={() => props.onReview()}>
            {t("changes.card.review")}
          </button>
        </div>
      </div>
      {discard.error ? <p className="changes-card-empty" role="alert">{discard.error}</p> : null}
      {skipped > 0 ? <p className="changes-card-empty">{tp("changes.skipped", skipped)}</p> : null}
      <div className="changes-card-rows">
        {changes.files.slice(0, ROW_CAP).map((file) => (
          <button
            key={file.path}
            type="button"
            className={"changes-row changes-row--" + file.status}
            data-testid={"changes-row-" + file.path}
            title={file.path}
            onClick={() => props.onFile(file.path)}
          >
            <span className="changes-row-path">
              <span className="changes-row-name">{baseName(file.path)}</span>
              {dirName(file.path) ? <span className="changes-row-dir">{dirName(file.path)}</span> : null}
            </span>
            {file.binary ? (
              <span className="changes-row-binary">{t("changes.binary")}</span>
            ) : (
              <span className="changes-row-stat">
                <span className="changes-add">{"+" + file.additions}</span>
                <span className="changes-del">{"−" + file.deletions}</span>
              </span>
            )}
          </button>
        ))}
        {hidden > 0 ? (
          <button type="button" className="changes-row changes-row--more" onClick={() => props.onReview()}>
            {t("changes.card.more", { count: hidden })}
          </button>
        ) : null}
      </div>
    </section>
  );
}
