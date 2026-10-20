import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import { useT } from "../i18n/I18nProvider";
// Module-level helpers below run outside React, so they translate through the store directly;
// the component keeps using the hook's t(). Aliased to avoid shadowing that local binding.
import { hasTranslation, translate } from "../i18n/i18n";
import { type JsonSchema } from "./SchemaForm";
import { deriveSettingsSections, type SectionDescriptor } from "./settingsSections";
import { SettingsNav } from "./SettingsNav";
import { SettingsSection } from "./SettingsSection";
import type { SessionsProjectScope } from "../sessions/SessionsManager";
import { SettingsTileGrid } from "./SettingsTileGrid";
import {
  serverSnapshotShellStack,
  snapshotShellStack,
  subscribeShellStack,
} from "../shellBreakpoint";
import { parseAppHash, setSettingsHash, setSettingsSectionHash } from "../scheduler/hashRoute";

// The project website (site/ in the repository, published on GitHub Pages).
const FOXXYCODE_SITE_URL = "https://hijera.github.io/foxxy-agent/";

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
    return {
      ok: false,
      error: e instanceof Error ? e.message : translate("settings.parseError"),
    };
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

/** Back arrow (lucide arrow-left) for the mobile section-detail header. */
function IconArrowLeft(props: { className?: string }) {
  return (
    <svg
      className={props.className}
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.9"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M19 12H5" />
      <path d="M12 19l-7-7 7-7" />
    </svg>
  );
}

/** The drawer's title while a row of a list section is open: the form it
 * shows ("Provider settings"), the section's own name for a list without one. */
function itemFormTitle(section: SectionDescriptor): string {
  const key = `settings.item.${section.id}`;
  return hasTranslation(key) ? translate(key) : section.label;
}

/** Whether the address keeps the history sidebar open (`?history=1`). */
function historyOpenInAddress(): boolean {
  const route = parseAppHash();
  return "historyOpen" in route && route.historyOpen;
}

export function Settings(props: {
  onClose: () => void;
  /** Called after the config is successfully saved so the app can re-fetch model metadata. */
  onConfigSaved?: () => void;
  /** Section id from the `#/settings/<section>` deep link (null = default/grid). */
  initialSection?: string | null;
  /** Reopen the onboarding form + guided tour (shown in the Appearance tab). */
  onRestartOnboarding?: () => void;
  /** The open row from the settings deep link. */
  initialItem?: string | null;
  /**
   * Workspace of the viewed session. The Subagents tab lists the definitions
   * of that workspace and writes its approvals there, because a receipt is
   * keyed by workspace and spawn_agent checks the session's own cwd. Undefined
   * with no session open: the server answers for its session default.
   */
  workspacePath?: string | undefined;
  /** The conversation on screen; the session table protects it from a delete. */
  activeSessionId?: string | undefined;
  /** Session ids the table removed, so the shell can drop them from History. */
  onSessionsDeleted?: ((ids: string[]) => void) | undefined;
  /** The History "this project only" scope, which the session table follows. */
  sessionsScope?: SessionsProjectScope | undefined;
  /** Reports labels edited here, so the drawer behind the panel agrees. */
  onSessionTagsChanged?: ((id: string, tags: string[]) => void) | undefined;
}) {
  const { t } = useT();
  const [schema, setSchema] = useState<JsonSchema | null>(null);
  const [doc, setDoc] = useState<Record<string, unknown>>({});
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<string>(props.initialSection ?? "");
  // Animation feedback: bump reloadKey to replay the form dissolve/reappear on
  // reload; reloading spins the refresh icon; justSaved pulses the save button.
  const [reloadKey, setReloadKey] = useState(0);
  const [reloading, setReloading] = useState(false);
  const [justSaved, setJustSaved] = useState(false);

  // On narrow shells the section picker is a tile grid (master) that opens one
  // section at a time (detail); `mobileDetailId` null means the grid is showing.
  const isMobileShell = useSyncExternalStore(
    subscribeShellStack,
    snapshotShellStack,
    serverSnapshotShellStack,
  );
  const [mobileDetailId, setMobileDetailId] = useState<string | null>(
    props.initialSection ?? null,
  );

  const sections = useMemo(() => deriveSettingsSections(schema), [schema]);
  const activeSection =
    sections.find((s) => s.id === activeTab) ?? sections[0] ?? null;
  const mobileSection = mobileDetailId
    ? sections.find((s) => s.id === mobileDetailId) ?? null
    : null;

  // Reflect the `#/settings/<section>` deep link (initial load and browser
  // back/forward) into local tab state; writing the hash below re-enters here
  // with the same value, so this is a no-op on self-initiated changes.
  const routeSection = props.initialSection ?? null;
  useEffect(() => {
    setActiveTab(routeSection ?? "");
    setMobileDetailId(routeSection);
  }, [routeSection]);

  // Selecting a section (desktop tab or mobile tile) anchors it in the URL.
  const selectSection = useCallback((id: string) => {
    setActiveTab(id);
    setMobileDetailId(id);
    setSettingsSectionHash(id);
  }, []);

  // Mobile back to the tile grid drops the section anchor.
  const backToGrid = useCallback(() => {
    setMobileDetailId(null);
    setSettingsHash();
  }, []);

  // The head is the way back, on every width. With a row of a list section
  // open (a provider, a model) it is titled after that form and its arrow
  // closes the form back onto the list; on the narrow shell, with no row open,
  // it names the section and its arrow goes back to the tiles.
  const [rowOpen, setRowOpen] = useState(false);
  const [closeRowSignal, setCloseRowSignal] = useState(0);
  const headSection = isMobileShell ? mobileSection : activeSection;
  const rowTitle = rowOpen && headSection ? itemFormTitle(headSection) : null;

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

  // Renders the content panel for a section, reusing the schema-present and
  // appearance-without-schema paths for both the desktop rail and the mobile
  // tile-grid detail view.
  const renderSectionBody = (section: SectionDescriptor | null) => {
    if (schema) {
      return (
        <div className="settings-scroll">
          <div
            className={`settings-body${reloadKey > 0 ? " settings-form-anim" : ""}`}
            key={reloadKey}
          >
            {section ? (
              <SettingsSection
                section={section}
                schema={schema}
                doc={doc}
                setDoc={setDoc}
                isMobileShell={isMobileShell}
                onRestartOnboarding={props.onRestartOnboarding}
                // The address names a row of one section. For the render in
                // which the tab has not caught up with a new address yet,
                // another section must not take that name for its own (it
                // would find no such row and rewrite the address to its list).
                routeItem={
                  section.id === (props.initialSection ?? "")
                    ? (props.initialItem ?? null)
                    : null
                }
                // Only the section the address names writes it back: in the
                // render where the tab lags a new address, the old section
                // would otherwise report its closed form and undo the jump.
                onRouteItemChange={
                  section.id === (props.initialSection ?? "")
                    ? (item) =>
                        setSettingsSectionHash(section.id, {
                          item,
                          historySidebar: historyOpenInAddress(),
                        })
                    : undefined
                }
                hideBackLink
                onEditingChange={setRowOpen}
                closeSignal={closeRowSignal}
                workspacePath={props.workspacePath}
                activeSessionId={props.activeSessionId}
                onSessionsDeleted={props.onSessionsDeleted}
                sessionsScope={props.sessionsScope}
                onSessionTagsChanged={props.onSessionTagsChanged}
              />
            ) : null}
          </div>
        </div>
      );
    }
    if (!loadErr) {
      // General (language picker), Appearance (theme picker) and Sessions (the
      // stored history over /foxxycode/sessions) are client-side content,
      // available before the config schema loads. Render them in the normal
      // scroll flow — NOT the centered `settings-scroll-placeholder` used for the
      // "Loading…" spinner, which shrinks and off-centers the content.
      if (
        section &&
        (section.kind === "appearance" ||
          section.kind === "general" ||
          section.kind === "sessions")
      ) {
        return (
          <div className="settings-scroll">
            <div className="settings-body">
              <SettingsSection
                section={section}
                schema={{ type: "object", properties: {} } as JsonSchema}
                doc={doc}
                setDoc={setDoc}
                onRestartOnboarding={props.onRestartOnboarding}
                activeSessionId={props.activeSessionId}
                onSessionsDeleted={props.onSessionsDeleted}
                sessionsScope={props.sessionsScope}
                onSessionTagsChanged={props.onSessionTagsChanged}
              />
            </div>
          </div>
        );
      }
      return (
        <div className="settings-scroll settings-scroll-placeholder">
          <p className="settings-muted">{t("settings.loading")}</p>
        </div>
      );
    }
    return null;
  };

  return (
    <aside
      className="sessions settings drawer"
      aria-label={t("settings.title")}
      data-testid="settings-screen"
      data-variant="drawer"
    >
      <div className="sessions-head">
        {rowTitle && headSection ? (
          <span className="settings-head-titlegroup">
            <button
              type="button"
              className="settings-head-back"
              aria-label={t("settings.array.backTo", {
                list: headSection.label,
              })}
              title={t("settings.array.backTo", { list: headSection.label })}
              data-testid="settings-head-back"
              onClick={() => setCloseRowSignal((n) => n + 1)}
            >
              <IconArrowLeft />
            </button>
            <span className="settings-head-section">{rowTitle}</span>
          </span>
        ) : isMobileShell && mobileSection ? (
          <span className="settings-head-titlegroup">
            <button
              type="button"
              className="settings-head-back"
              aria-label={t("settings.backToSections")}
              title={t("settings.backToSections")}
              data-testid="settings-head-back"
              onClick={backToGrid}
            >
              <IconArrowLeft />
            </button>
            <span className="settings-head-section">{mobileSection.label}</span>
          </span>
        ) : (
          <span>{t("settings.title")}</span>
        )}
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

      {loadErr || error || message ? (
        <div className="settings-lead-pane">
          {loadErr ? (
            <p className="settings-error">
              {t("settings.failedToLoad", { error: loadErr })}
            </p>
          ) : null}
          {error ? <p className="settings-error">{error}</p> : null}
          {message ? <p className="settings-ok">{message}</p> : null}
        </div>
      ) : null}

      <div className="settings-stack">
        {isMobileShell ? (
          mobileSection ? (
            <div className="settings-mobile-detail">
              {renderSectionBody(mobileSection)}
            </div>
          ) : (
            <SettingsTileGrid sections={sections} onSelect={selectSection} />
          )
        ) : (
          <div className="settings-tabs-layout">
            <SettingsNav
              sections={sections}
              active={activeSection ? activeSection.id : ""}
              onSelect={selectSection}
            />
            {renderSectionBody(activeSection)}
          </div>
        )}

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
          <a
            className="settings-footer-link"
            href={FOXXYCODE_SITE_URL}
            target="_blank"
            rel="noopener noreferrer"
            data-testid="settings-site-link"
          >
            {t("settings.siteLink")}
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
