import { useCallback, useEffect, useMemo, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { type JsonSchema } from "./SchemaForm";
import { deriveSettingsSections, type SectionDescriptor } from "./settingsSections";
import { SettingsNav } from "./SettingsNav";
import { SettingsSection } from "./SettingsSection";

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

function IconSave(props: { className?: string }) {
  return (
    <svg
      className={props.className}
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z" />
      <polyline points="17 21 17 13 7 13 7 21" />
      <polyline points="7 3 7 8 15 8" />
    </svg>
  );
}

function IconRefresh(props: { className?: string }) {
  return (
    <svg
      className={props.className}
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    </svg>
  );
}

export function Settings(props: {
  onClose: () => void;
  /** Called after the config is successfully saved so the app can re-fetch model metadata. */
  onConfigSaved?: () => void;
}) {
  const { t } = useT();
  const [schema, setSchema] = useState<JsonSchema | null>(null);
  const [doc, setDoc] = useState<Record<string, unknown>>({});
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<string>("");
  // Animation feedback: bump reloadKey to replay the form dissolve/reappear on
  // reload; reloading spins the refresh icon; justSaved pulses the save button.
  const [reloadKey, setReloadKey] = useState(0);
  const [reloading, setReloading] = useState(false);
  const [justSaved, setJustSaved] = useState(false);
  // Section id whose Save button just succeeded (drives the per-section save pulse).
  const [savedSection, setSavedSection] = useState<string | null>(null);

  const sections = useMemo(() => deriveSettingsSections(schema), [schema]);
  const activeSection =
    sections.find((s) => s.id === activeTab) ?? sections[0] ?? null;

  const load = useCallback(async () => {
    setLoadErr(null);
    const [sRes, cRes] = await Promise.all([
      readJSON<JsonSchema>("/foxxycode/config/schema"),
      readJSON<Record<string, unknown>>("/foxxycode/config"),
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

  // Reload with visible feedback: spin the refresh icon and replay the form
  // dissolve/reappear animation (key bump remounts the content) while re-fetching.
  const onReload = useCallback(async () => {
    setReloading(true);
    setReloadKey((k) => k + 1);
    try {
      await Promise.all([
        load(),
        new Promise((r) => window.setTimeout(r, 500)),
      ]);
    } finally {
      setReloading(false);
    }
  }, [load]);

  const onSave = useCallback(async () => {
    setBusy(true);
    setMessage(null);
    setError(null);
    try {
      const body = JSON.stringify(doc);
      const v = await fetch("/foxxycode/config/validate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body,
      });
      const vj = (await v.json()) as ValidateResponse;
      if (!vj.ok) {
        setError(vj.error || t("settings.validationFailed"));
        setBusy(false);
        return;
      }
      const p = await fetch("/foxxycode/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body,
      });
      const pj = (await p.json()) as ValidateResponse;
      if (!p.ok || !pj.ok) {
        setError(pj.error || t("settings.saveFailed", { status: p.status }));
        setBusy(false);
        return;
      }
      setMessage(t("settings.savedAll"));
      setJustSaved(true);
      window.setTimeout(() => setJustSaved(false), 1100);
      props.onConfigSaved?.();
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("settings.requestFailed"));
    } finally {
      setBusy(false);
    }
  }, [doc, load, props, t]);

  // Persist only one section: overlay this section's values onto the latest
  // on-disk config and PUT that, so saving one section does not also commit
  // unsaved edits made in other sections.
  const onSaveSection = useCallback(
    async (section: SectionDescriptor) => {
      const keys =
        section.kind === "group"
          ? section.childKeys ?? []
          : section.kind === "appearance"
            ? []
            : [section.schemaKey ?? section.id];
      if (keys.length === 0) {
        return;
      }
      setBusy(true);
      setMessage(null);
      setError(null);
      try {
        const fresh = await readJSON<Record<string, unknown>>("/foxxycode/config");
        if (!fresh.ok || !fresh.data) {
          setError(fresh.error || "config");
          setBusy(false);
          return;
        }
        const merged: Record<string, unknown> = { ...fresh.data };
        for (const k of keys) {
          merged[k] = doc[k];
        }
        const body = JSON.stringify(merged);
        const v = await fetch("/foxxycode/config/validate", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body,
        });
        const vj = (await v.json()) as ValidateResponse;
        if (!vj.ok) {
          setError(vj.error || t("settings.validationFailed"));
          setBusy(false);
          return;
        }
        const p = await fetch("/foxxycode/config", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body,
        });
        const pj = (await p.json()) as ValidateResponse;
        if (!p.ok || !pj.ok) {
          setError(pj.error || t("settings.saveFailed", { status: p.status }));
          setBusy(false);
          return;
        }
        setMessage(t("settings.savedSection", { section: section.label }));
        setSavedSection(section.id);
        window.setTimeout(() => setSavedSection(null), 1100);
        props.onConfigSaved?.();
      } catch (e) {
        setError(e instanceof Error ? e.message : t("settings.requestFailed"));
      } finally {
        setBusy(false);
      }
    },
    [doc, props, t],
  );

  return (
    <aside
      className="sessions settings drawer"
      aria-label={t("settings.title")}
      data-testid="settings-screen"
      data-variant="drawer"
    >
      <div className="sessions-head">
        <span>{t("settings.title")}</span>
        <button
          type="button"
          className="sessions-close"
          aria-label={t("settings.close")}
          data-testid="settings-drawer-close"
          onClick={props.onClose}
        >
          ×
        </button>
      </div>

      <div className="settings-lead-pane">
        <p className="settings-lead">{t("settings.lead")}</p>
        {loadErr ? (
          <p className="settings-error">{t("settings.failedToLoad", { error: loadErr })}</p>
        ) : null}
        {error ? <p className="settings-error">{error}</p> : null}
        {message ? <p className="settings-ok">{message}</p> : null}
      </div>

      <div className="settings-stack">
        <div className="settings-tabs-layout">
          <SettingsNav
            sections={sections}
            active={activeSection ? activeSection.id : ""}
            onSelect={setActiveTab}
          />
          {schema ? (
            <div className="settings-scroll">
              <div
                className={`settings-body${reloadKey > 0 ? " settings-form-anim" : ""}`}
                key={reloadKey}
              >
                {activeSection && activeSection.kind !== "appearance" ? (
                  <div className="settings-section-actions">
                    <button
                      type="button"
                      className={`settings-btn settings-btn-primary settings-section-save${
                        savedSection === activeSection.id ? " is-saved" : ""
                      }`}
                      data-testid="settings-section-save"
                      disabled={busy}
                      title={t("settings.saveSectionTitle", {
                        section: activeSection.label,
                      })}
                      onClick={() => void onSaveSection(activeSection)}
                    >
                      {t("settings.saveSection")}
                    </button>
                  </div>
                ) : null}
                {activeSection ? (
                  <SettingsSection
                    section={activeSection}
                    schema={schema}
                    doc={doc}
                    setDoc={setDoc}
                  />
                ) : null}
              </div>
            </div>
          ) : !loadErr ? (
            <div className="settings-scroll settings-scroll-placeholder">
              {activeSection && activeSection.kind === "appearance" ? (
                <div className="settings-body">
                  <SettingsSection
                    section={activeSection}
                    schema={{ type: "object", properties: {} } as JsonSchema}
                    doc={doc}
                    setDoc={setDoc}
                  />
                </div>
              ) : (
                <p className="settings-muted">{t("settings.loading")}</p>
              )}
            </div>
          ) : null}
        </div>

        <div className="scheduler-drawer-footer settings-footer-actions">
          <a
            className="settings-footer-link"
            href="/docs/"
            target="_blank"
            rel="noopener"
            data-testid="settings-api-docs-link"
          >
            {t("settings.apiDocs")}
          </a>
          <button
            type="button"
            className="settings-btn settings-btn-icon"
            data-testid="settings-reload"
            disabled={busy || reloading}
            title={t("settings.reloadTitle")}
            aria-label={t("settings.reloadAriaLabel")}
            onClick={() => void onReload()}
          >
            <IconRefresh
              className={`settings-footer-icon-svg${reloading ? " settings-icon-spin" : ""}`}
            />
          </button>
          <button
            type="button"
            className={`settings-btn settings-btn-primary settings-btn-icon${justSaved ? " is-saved" : ""}`}
            data-testid="settings-save"
            disabled={busy || !schema}
            title={t("settings.saveAllTitle")}
            aria-label={t("settings.saveAllAriaLabel")}
            onClick={() => void onSave()}
          >
            <IconSave className="settings-footer-icon-svg" />
          </button>
        </div>
      </div>
    </aside>
  );
}
