import { useEffect, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { fetchMiniAppFile } from "./api";
import type { MiniAppDocument, MiniAppFile, MiniAppRun } from "./types";

function ResultFile(props: { runId: string; file: MiniAppFile }) {
  const { t } = useT();
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const file = props.file;
  const preview =
    (file.renderer ?? "auto") !== "download" &&
    /^(audio|video|image)\//.test(file.media_type) &&
    file.media_type !== "image/svg+xml";
  useEffect(() => {
    if (!preview) return;
    let cancelled = false;
    let objectUrl: string | undefined;
    void fetchMiniAppFile(props.runId, file.id).then((result) => {
      if (cancelled) return;
      if (result.ok) {
        objectUrl = URL.createObjectURL(result.data);
        setUrl(objectUrl);
      } else setError(result.message);
    });
    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [props.runId, file.id, preview]);
  useEffect(
    () => () => {
      if (url) URL.revokeObjectURL(url);
    },
    [url],
  );
  const download = async () => {
    setBusy(true);
    setError(null);
    const result = await fetchMiniAppFile(props.runId, file.id);
    if (result.ok) {
      const objectUrl = URL.createObjectURL(result.data);
      const anchor = document.createElement("a");
      anchor.href = objectUrl;
      anchor.download = file.name;
      anchor.click();
      window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
    } else setError(result.message);
    setBusy(false);
  };
  return (
    <section className="miniapps-output-file">
      <h4>{file.title || file.name}</h4>
      <small>
        {file.name} · {Math.ceil(file.size_bytes / 1024)} KB
      </small>
      {url && preview ? (
        file.media_type.startsWith("audio/") ? (
          <audio controls src={url} aria-label={file.title || file.name} />
        ) : file.media_type.startsWith("video/") ? (
          <video controls src={url} aria-label={file.title || file.name} />
        ) : (
          <img src={url} alt={file.title || file.name} />
        )
      ) : null}
      <button
        type="button"
        className="miniapps-secondary"
        disabled={busy}
        onClick={() => void download()}
      >
        {busy
          ? t("miniapps.files.loading")
          : t("miniapps.files.download", { name: file.name })}
      </button>
      {error ? (
        <p role="alert" className="miniapps-error">
          {error}
        </p>
      ) : null}
    </section>
  );
}

export function MiniAppResults(props: {
  app: MiniAppDocument;
  run: MiniAppRun;
}) {
  const { t } = useT();
  const values =
    props.run.outputs && typeof props.run.outputs === "object"
      ? (props.run.outputs as Record<string, unknown>)
      : null;
  const files = props.run.files ?? [];
  const definitions = props.app.outputs ?? [];
  const textOutputs = values
    ? Object.entries(values).filter(
        ([id]) => !files.some((file) => file.output_id === id),
      )
    : [];
  if (
    files.length === 0 &&
    textOutputs.length === 0 &&
    props.run.result === undefined
  )
    return null;
  return (
    <section className="miniapps-result">
      <h3>{t("miniapps.result")}</h3>
      {files.map((file) => (
        <ResultFile
          key={file.id}
          runId={props.run.run_id || props.run.id || ""}
          file={file}
        />
      ))}
      {textOutputs.map(([id, value]) => {
        const definition = definitions.find((item) => item.id === id);
        const rows =
          definition?.renderer === "table" && Array.isArray(value)
            ? (value.filter(
                (item) =>
                  item && typeof item === "object" && !Array.isArray(item),
              ) as Record<string, unknown>[])
            : null;
        const columns = rows
          ? Array.from(new Set(rows.flatMap((row) => Object.keys(row)))).slice(
              0,
              20,
            )
          : [];
        return (
          <section key={id}>
            <h4>{definition?.title || id}</h4>
            {rows ? (
              <div className="miniapps-result-table">
                <table>
                  <thead>
                    <tr>
                      {columns.map((column) => (
                        <th key={column}>{column}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rows.slice(0, 100).map((row, index) => (
                      <tr key={index}>
                        {columns.map((column) => (
                          <td key={column}>
                            {typeof row[column] === "object"
                              ? JSON.stringify(row[column])
                              : String(row[column] ?? "")}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <pre className="miniapps-report">
                {typeof value === "string"
                  ? value
                  : JSON.stringify(value, null, 2)}
              </pre>
            )}
          </section>
        );
      })}
      {!values && files.length === 0 ? (
        <pre className="miniapps-report">
          {JSON.stringify(props.run.result, null, 2)}
        </pre>
      ) : null}
    </section>
  );
}
