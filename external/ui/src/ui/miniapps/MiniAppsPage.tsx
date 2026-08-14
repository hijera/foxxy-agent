import { useCallback, useEffect, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  getAuthoringSource,
  getMiniApp,
  getMiniAppDraft,
  getMiniAppRelease,
  listMiniApps,
  listRuns,
} from "./api";
import { DistillationWorkspace } from "./DistillationWorkspace";
import { MiniAppEditor } from "./MiniAppEditor";
import { MiniAppsCatalog } from "./MiniAppsCatalog";
import { MiniAppRunner } from "./MiniAppRunner";
import { RunHistory } from "./RunHistory";
import type { MiniAppCatalogEntry, MiniAppDocument, MiniAppRun } from "./types";

export function MiniAppsPage(props: {
  selectedAppId?: string | null;
  sessionId?: string;
  sessionEligible?: boolean;
  onNavigate: (appId?: string | null) => void;
  onClose: () => void;
}) {
  const { t } = useT();
  const [apps, setApps] = useState<MiniAppCatalogEntry[]>([]);
  const [selected, setSelected] = useState<MiniAppDocument | null>(null);
  const [sourceEvidence, setSourceEvidence] = useState<Record<
    string,
    unknown
  > | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [distilling, setDistilling] = useState(false);
  const [running, setRunning] = useState<{
    app: MiniAppDocument;
    released: boolean;
  } | null>(null);
  const [runs, setRuns] = useState<MiniAppRun[]>([]);
  const [verifiedRevision, setVerifiedRevision] = useState<string | null>(null);
  const [validatedRevision, setValidatedRevision] = useState<string | null>(
    null,
  );
  const [sanitizedRevision, setSanitizedRevision] = useState<string | null>(
    null,
  );

  const loadApps = useCallback(async () => {
    setLoading(true);
    const result = await listMiniApps();
    if (result.ok) {
      setApps(result.data);
      setError(null);
    } else setError(result.message);
    setLoading(false);
  }, []);
  useEffect(() => {
    void loadApps();
  }, [loadApps]);

  useEffect(() => {
    const id = (props.selectedAppId || "").trim();
    if (!id) {
      setSelected(null);
      setSourceEvidence(null);
      setRuns([]);
      setVerifiedRevision(null);
      setValidatedRevision(null);
      setSanitizedRevision(null);
      return;
    }
    let cancelled = false;
    void (async () => {
      const result = await getMiniAppDraft(id);
      const fallback = result.ok ? result : await getMiniApp(id);
      if (!cancelled) {
        if (fallback.ok) {
          setSelected(fallback.data);
          setVerifiedRevision(null);
          setValidatedRevision(null);
          setSanitizedRevision(null);
          setError(null);
          const [source, history] = await Promise.all([
            getAuthoringSource(id),
            listRuns(id),
          ]);
          if (!cancelled) {
            if (source.ok) setSourceEvidence(source.data);
            if (history.ok) setRuns(history.data);
          }
        } else setError(fallback.message);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [props.selectedAppId]);

  const open = (id: string) => props.onNavigate(id);
  const completeDistillation = (id?: string) => {
    setDistilling(false);
    void loadApps();
    if (id) props.onNavigate(id);
  };
  const canCreate = !!props.sessionId?.trim() && props.sessionEligible === true;
  const selectedCatalog = apps.find(
    (app) => app.id === (props.selectedAppId || "").trim(),
  );
  const releasedVersion =
    selectedCatalog?.state === "released" ? selectedCatalog.version : undefined;

  const runReleased = async () => {
    if (!selected || !releasedVersion) return;
    const result = await getMiniAppRelease(selected.id, releasedVersion);
    if (result.ok) setRunning({ app: result.data, released: true });
    else setError(result.message);
  };

  if (distilling && props.sessionId)
    return (
      <main className="miniapps-page">
        <DistillationWorkspace
          sessionId={props.sessionId}
          onComplete={completeDistillation}
          onCancel={() => setDistilling(false)}
        />
      </main>
    );
  if (running && selected)
    return (
      <main className="miniapps-page">
        <MiniAppRunner
          app={running.app}
          released={running.released}
          onClose={() => setRunning(null)}
          onCompleted={(run) => {
            if (run.status === "succeeded") {
              setVerifiedRevision(selected.revision || null);
            }
            void listRuns(selected.id).then((history) => {
              if (history.ok) setRuns(history.data);
            });
          }}
        />
        <RunHistory runs={runs} />
      </main>
    );

  return (
    <main className="miniapps-page" data-testid="miniapps-page">
      <div className="miniapps-page-header">
        <div>
          <p className="miniapps-eyebrow">{t("miniapps.eyebrow")}</p>
          <h1>{t("miniapps.title")}</h1>
          <p className="miniapps-lead">{t("miniapps.pageDescription")}</p>
        </div>
        <button
          type="button"
          className="miniapps-quiet"
          onClick={props.onClose}
        >
          {t("miniapps.close")}
        </button>
      </div>
      <div className="miniapps-layout">
        <MiniAppsCatalog
          apps={apps}
          selectedId={props.selectedAppId || null}
          loading={loading}
          error={error}
          canCreate={canCreate}
          onCreate={() => setDistilling(true)}
          onSelect={open}
        />
        {selected ? (
          <MiniAppEditor
            initial={selected}
            sourceEvidence={sourceEvidence}
            releasedVersion={releasedVersion}
            verifiedRevision={verifiedRevision}
            validatedRevision={validatedRevision}
            sanitizedRevision={sanitizedRevision}
            onValidated={(revision) => setValidatedRevision(revision)}
            onSanitized={(revision) => setSanitizedRevision(revision)}
            onChecksReset={() => {
              setVerifiedRevision(null);
              setValidatedRevision(null);
              setSanitizedRevision(null);
            }}
            onSaved={(app) => setSelected(app)}
            onRun={(app, release) => setRunning({ app, released: release })}
            onRunReleased={() => void runReleased()}
          />
        ) : (
          <section className="miniapps-detail-empty">
            <strong>{t("miniapps.selectTitle")}</strong>
            <p>{t("miniapps.selectDescription")}</p>
          </section>
        )}
      </div>
    </main>
  );
}
