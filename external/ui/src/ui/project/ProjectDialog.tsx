import { useEffect, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  fetchRecentProjects,
  pickFolder,
  putProject,
  type ProjectInfo,
  type RecentProject,
} from "./projectApi";

export function ProjectDialog(props: {
  open: boolean;
  project: ProjectInfo | null;
  onClose: () => void;
  onOpened: (info: ProjectInfo) => void;
}) {
  const { t } = useT();
  const [path, setPath] = useState("");
  const [recent, setRecent] = useState<RecentProject[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!props.open) return;
    setPath("");
    setError(null);
    void fetchRecentProjects().then(setRecent);
  }, [props.open]);

  if (!props.open) {
    return null;
  }

  const nativePicker = !!props.project?.native_picker;
  const currentPath = props.project?.path || "";

  const browse = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await pickFolder();
      if (!res) {
        setError(t("project.pickFailed"));
        return;
      }
      if ("unavailable" in res) {
        setError(t("project.pickerUnavailable"));
        return;
      }
      if (!res.cancelled && res.path) {
        setPath(res.path);
      }
    } finally {
      setBusy(false);
    }
  };

  const openProject = async (target?: string) => {
    const p = (target !== undefined ? target : path).trim();
    if (!p) return;
    setBusy(true);
    setError(null);
    try {
      const res = await putProject(p);
      if (!res.ok) {
        setError(res.error);
        return;
      }
      props.onOpened(res.info);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="project-dialog-host" data-testid="project-dialog">
      <button
        type="button"
        className="backdrop is-open"
        aria-label={t("project.close")}
        onClick={props.onClose}
      />
      <div
        className="project-dialog-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="project-dialog-title"
        onKeyDown={(ev) => {
          if (ev.key === "Escape") {
            props.onClose();
          }
        }}
      >
        <div className="project-dialog-head">
          <h2 id="project-dialog-title">{t("project.dialogTitle")}</h2>
          {currentPath ? (
            <p
              className="project-dialog-current"
              title={currentPath}
              data-testid="project-current-path"
            >
              {t("project.currentLabel")} {currentPath}
            </p>
          ) : null}
        </div>

        <div className="project-dialog-form">
          <div className="project-dialog-path-row">
            <input
              className="project-dialog-input"
              value={path}
              onChange={(ev) => setPath(ev.target.value)}
              placeholder={t("project.pathPlaceholder")}
              data-testid="project-path-input"
              onKeyDown={(ev) => {
                if (ev.key === "Enter") {
                  void openProject();
                }
              }}
            />
            {nativePicker ? (
              <button
                type="button"
                className="project-dialog-secondary"
                onClick={() => void browse()}
                disabled={busy}
                data-testid="project-browse"
              >
                {t("project.browse")}
              </button>
            ) : null}
          </div>

          {error ? (
            <div className="project-dialog-error" data-testid="project-error">
              {error}
            </div>
          ) : null}

          {recent.length > 0 ? (
            <div className="project-dialog-recent">
              <div className="project-dialog-recent-label">
                {t("project.recentLabel")}
              </div>
              {recent.map((r) => (
                <button
                  key={r.path}
                  type="button"
                  className={[
                    "project-dialog-recent-row",
                    r.exists ? "" : "project-dialog-recent-row--missing",
                  ]
                    .filter(Boolean)
                    .join(" ")}
                  title={r.exists ? r.path : `${r.path} — ${t("project.missing")}`}
                  data-testid={`project-recent-${r.name}`}
                  disabled={busy}
                  onClick={() => {
                    setPath(r.path);
                    if (r.exists) {
                      void openProject(r.path);
                    }
                  }}
                >
                  <span className="project-dialog-recent-name">{r.name}</span>
                  <span className="project-dialog-recent-path">{r.path}</span>
                </button>
              ))}
            </div>
          ) : null}
        </div>

        <div className="project-dialog-actions">
          <button
            type="button"
            className="project-dialog-secondary"
            onClick={props.onClose}
            disabled={busy}
            data-testid="project-cancel"
          >
            {t("project.cancel")}
          </button>
          <button
            type="button"
            className="project-dialog-primary"
            onClick={() => void openProject()}
            disabled={busy || !path.trim()}
            data-testid="project-open"
          >
            {t("project.open")}
          </button>
        </div>
      </div>
    </div>
  );
}
