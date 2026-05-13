import { useCallback, useEffect, useState } from "react";
import { SchemaForm, type JsonSchema } from "./SchemaForm";

type ValidateResponse = { ok: boolean; error?: string };

async function readJSON<T>(path: string): Promise<{ ok: boolean; data?: T; error?: string }> {
  const res = await fetch(path);
  if (!res.ok) {
    return { ok: false, error: `${res.status}` };
  }
  try {
    const data = (await res.json()) as T;
    return { ok: true, data };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "parse" };
  }
}

export function Settings(props: { onClose: () => void }) {
  const [schema, setSchema] = useState<JsonSchema | null>(null);
  const [doc, setDoc] = useState<Record<string, unknown>>({});
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoadErr(null);
    const [sRes, cRes] = await Promise.all([
      readJSON<JsonSchema>("/coddy/config/schema"),
      readJSON<Record<string, unknown>>("/coddy/config"),
    ]);
    if (!sRes.ok || !sRes.data) {
      setLoadErr(sRes.error || "schema");
      return;
    }
    if (!cRes.ok || !cRes.data) {
      setLoadErr(cRes.error || "config");
      return;
    }
    setSchema(sRes.data);
    setDoc(cRes.data);
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const onSave = useCallback(async () => {
    setBusy(true);
    setMessage(null);
    setError(null);
    try {
      const body = JSON.stringify(doc);
      const v = await fetch("/coddy/config/validate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body,
      });
      const vj = (await v.json()) as ValidateResponse;
      if (!vj.ok) {
        setError(vj.error || "validation failed");
        setBusy(false);
        return;
      }
      const p = await fetch("/coddy/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body,
      });
      const pj = (await p.json()) as ValidateResponse;
      if (!p.ok || !pj.ok) {
        setError(pj.error || `save failed (${p.status})`);
        setBusy(false);
        return;
      }
      setMessage("Saved. In-process config reloaded.");
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "request failed");
    } finally {
      setBusy(false);
    }
  }, [doc, load]);

  return (
    <aside
      className="sessions settings drawer"
      aria-label="Settings"
      data-testid="settings-screen"
      data-variant="drawer"
    >
      <div className="sessions-head">
        <span>Settings</span>
        <button
          type="button"
          className="sessions-close"
          aria-label="Close settings"
          data-testid="settings-drawer-close"
          onClick={props.onClose}
        >
          ×
        </button>
      </div>

      <div className="settings-lead-pane">
        <p className="settings-lead">
          Edit configuration from the live JSON schema. Secrets (API keys) are shown in full -
          use only on trusted networks.
        </p>
        {loadErr ? (
          <p className="settings-error">Failed to load: {loadErr}</p>
        ) : null}
        {error ? <p className="settings-error">{error}</p> : null}
        {message ? <p className="settings-ok">{message}</p> : null}
      </div>

      <div className="settings-stack">
        {schema ? (
          <div className="settings-scroll">
            <div className="settings-body">
              <SchemaForm schema={schema} value={doc} onChange={setDoc} />
            </div>
          </div>
        ) : !loadErr ? (
          <div className="settings-scroll settings-scroll-placeholder">
            <p className="settings-muted">Loading…</p>
          </div>
        ) : null}

        <div className="scheduler-drawer-footer settings-footer-actions">
          <button
            type="button"
            className="settings-btn"
            data-testid="settings-close"
            onClick={props.onClose}
          >
            Close
          </button>
          <button
            type="button"
            className="settings-btn"
            data-testid="settings-reload"
            disabled={busy}
            onClick={() => void load()}
          >
            Reload
          </button>
          <button
            type="button"
            className="settings-btn settings-btn-primary"
            data-testid="settings-save"
            disabled={busy || !schema}
            onClick={() => void onSave()}
          >
            Save
          </button>
        </div>
      </div>
    </aside>
  );
}
