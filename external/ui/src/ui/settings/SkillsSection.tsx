import { useCallback, useEffect, useRef, useState } from "react";
import { SchemaForm, IconTrash, type JsonSchema, type FieldOverride } from "./SchemaForm";
import { LegendWithHint } from "./FieldHint";
import { MarketplacesEditor } from "./MarketplacesEditor";
import { sessionHeaders, withMarketplaceWorkspace } from "./marketplaces";
import { SwitchField } from "./SwitchField";
import { useT } from "../i18n/I18nProvider";
import { tSchemaText } from "../i18n/schemaStrings";
import { filterInstallableMatches } from "./installableMatches";
import { useEscapeCloses } from "../components/useEscapeCloses";

// Cap the install dropdown so a broad query never floods the menu; anything
// beyond this is summarized as a "+N more" hint that invites a narrower search.
const INSTALL_MENU_LIMIT = 10;

type InstalledSkill = {
  name: string;
  description: string;
  file_path: string;
  enabled: boolean;
  version?: string;
  source?: string;
  readonly?: boolean;
};

type SkillUpdate = {
  name: string;
  source: string;
  version: string;
  latest: string;
  update_available: boolean;
};

// workspacePath names the folder ${CWD} in skills.dirs resolves against, so the
// project skills listed are those of the chat's workspace, like Subagents.
async function fetchInstalled(
  sessionId: string | undefined,
  workspacePath: string | undefined,
): Promise<InstalledSkill[]> {
  const res = await fetch(
    withMarketplaceWorkspace("/foxxycode/skills", workspacePath),
    { headers: sessionHeaders(sessionId) },
  );
  if (!res.ok) return [];
  const data = (await res.json()) as { items?: InstalledSkill[] };
  return data.items ?? [];
}

// The update check, the install search, an update and an install go to the
// viewed chat's workspace, like the marketplaces list they follow: a project
// marketplace approved there is offered, and one held there is neither checked
// nor updated from.
async function fetchUpdates(
  sessionId: string | undefined,
  workspacePath: string | undefined,
): Promise<SkillUpdate[]> {
  const res = await fetch(
    withMarketplaceWorkspace("/foxxycode/skills/updates", workspacePath),
    { headers: sessionHeaders(sessionId) },
  );
  if (!res.ok) return [];
  const data = (await res.json()) as { items?: SkillUpdate[] };
  return data.items ?? [];
}

type AvailablePlugin = {
  name: string;
  description: string;
  version?: string;
  source: string;
  installed: boolean;
};

async function fetchAvailable(
  sessionId: string | undefined,
  workspacePath: string | undefined,
): Promise<AvailablePlugin[]> {
  const res = await fetch(
    withMarketplaceWorkspace("/foxxycode/skills/available", workspacePath),
    { headers: sessionHeaders(sessionId) },
  );
  if (!res.ok) return [];
  const data = (await res.json()) as { items?: AvailablePlugin[] };
  return data.items ?? [];
}

async function apiSend(
  path: string,
  method: "POST" | "DELETE",
  body?: unknown,
  sessionId?: string | undefined,
): Promise<{ ok: boolean; error?: string }> {
  const init: RequestInit = {
    method,
    headers: sessionHeaders(sessionId, body !== undefined),
  };
  if (body !== undefined) init.body = JSON.stringify(body);
  const res = await fetch(path, init);
  if (!res.ok) {
    try {
      const j = (await res.json()) as { error?: { message?: string } };
      return { ok: false, error: j.error?.message || `HTTP ${res.status}` };
    } catch {
      return { ok: false, error: `HTTP ${res.status}` };
    }
  }
  return { ok: true };
}

// Download-to-tray glyph for the "download update" action.
function IconDownload() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none"
      stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 3v12" />
      <polyline points="7 10 12 15 17 10" />
      <path d="M5 21h14" />
    </svg>
  );
}

/**
 * SkillsSection is the combined Skills tab: the schema-driven `skills` editor
 * (extra directories, the project marketplace trust policy), the marketplaces
 * list (MarketplacesEditor, API-driven: ~/.foxxycode/marketplaces.json and the
 * workspace's .foxxycode/marketplaces.json, with a per-entry and a Sync-all
 * button and the trust shield), and the installed-skills list with versions,
 * an iOS-style enable switch, a Download-update action when a newer version
 * exists, and a Delete action (disabled for bundled read-only skills).
 */
export function SkillsSection(props: {
  schema: JsonSchema;
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  workspacePath?: string | undefined;
  /** The viewed session, whose workspace's marketplaces are listed. */
  activeSessionId?: string | undefined;
}) {
  const { schema, value, onChange } = props;
  const workspacePath = props.workspacePath;
  const { t } = useT();
  const [installed, setInstalled] = useState<InstalledSkill[]>([]);
  const [updates, setUpdates] = useState<Record<string, SkillUpdate>>({});
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  // Marketplace browse/install control.
  const [available, setAvailable] = useState<AvailablePlugin[] | null>(null);
  const [availableLoading, setAvailableLoading] = useState(false);
  // Bumped whenever the cached list is dropped, so a fetch that was already on
  // its way cannot bring back a list from before the change.
  const availableRequest = useRef(0);
  const [installQuery, setInstallQuery] = useState("");
  const [installBusy, setInstallBusy] = useState<Record<string, boolean>>({});
  // Name of a just-installed skill to briefly highlight in the list. We do not
  // scroll to it: the floating install menu never reflows the list, so the
  // user stays exactly where they are; the status line and the flash confirm
  // the install without a jarring jump.
  const [justInstalled, setJustInstalled] = useState<string | null>(null);

  const invalidateAvailable = useCallback(() => {
    availableRequest.current += 1;
    setAvailable(null);
    setAvailableLoading(false);
  }, []);

  // The installable list is fetched once and kept, but the server reads it from
  // the *saved* sources. Settings replaces the whole document after Save (and an
  // edit replaces the array), so a new `sources` array means the cached list may
  // be stale: drop it, and the next focus or keystroke in the search fetches it
  // again. Nothing is fetched here - a git source is cloned for every request.
  useEffect(() => {
    invalidateAvailable();
  }, [value, workspacePath, props.activeSessionId, invalidateAvailable]);

  // firstLoad guards the "Loading…" placeholder so a refresh never unmounts the
  // list (which would collapse height and jump the scroll to the top).
  // A list asked for a workspace left since (Settings open while another
  // folder is picked) must not paint over the list of the current one.
  const installedGenRef = useRef(0);
  const sessionId = props.activeSessionId;
  const loadInstalled = useCallback(async (firstLoad = false) => {
    const gen = ++installedGenRef.current;
    if (firstLoad) setLoading(true);
    const rows = await fetchInstalled(sessionId, workspacePath);
    if (gen !== installedGenRef.current) return;
    setInstalled(rows);
    // The latest load ends the placeholder, whichever call started it.
    setLoading(false);
  }, [sessionId, workspacePath]);

  const updatesGenRef = useRef(0);
  const refreshUpdates = useCallback(async () => {
    const gen = ++updatesGenRef.current;
    const ups = await fetchUpdates(sessionId, workspacePath);
    if (gen !== updatesGenRef.current) return {};
    const map: Record<string, SkillUpdate> = {};
    for (const u of ups) map[u.name] = u;
    setUpdates(map);
    return map;
  }, [sessionId, workspacePath]);

  useEffect(() => {
    void loadInstalled(true);
  }, [loadInstalled]);

  // After an install, briefly flash the new row so it is easy to spot, then
  // clear the flag. No scroll — the list position is left untouched.
  useEffect(() => {
    if (!justInstalled) return;
    const t = window.setTimeout(() => setJustInstalled(null), 2400);
    return () => window.clearTimeout(t);
  }, [justInstalled]);

  const onToggle = (skill: InstalledSkill) => {
    setBusy((p) => ({ ...p, [skill.name]: true }));
    setError(null);
    void (async () => {
      const action = skill.enabled ? "disable" : "enable";
      const res = await apiSend(`/foxxycode/skills/${encodeURIComponent(skill.name)}/${action}`, "POST");
      if (!res.ok) {
        setError(
          res.error ||
            t(skill.enabled ? "settings.skills.disableFailed" : "settings.skills.enableFailed"),
        );
      } else {
        await loadInstalled();
      }
      setBusy((p) => ({ ...p, [skill.name]: false }));
    })();
  };

  const onRemove = (skill: InstalledSkill) => {
    setBusy((p) => ({ ...p, [skill.name]: true }));
    setError(null);
    void (async () => {
      // The same workspace the list was read for: a project skill of the
      // chat's folder is found there, not in the server's default cwd.
      const res = await apiSend(
        withMarketplaceWorkspace(
          `/foxxycode/skills/${encodeURIComponent(skill.name)}`,
          workspacePath,
        ),
        "DELETE",
        undefined,
        sessionId,
      );
      if (!res.ok) {
        setError(res.error || t("settings.skills.deleteFailed"));
      } else {
        await loadInstalled();
      }
      setBusy((p) => ({ ...p, [skill.name]: false }));
    })();
  };

  const onUpdateSkill = (skill: InstalledSkill) => {
    setBusy((p) => ({ ...p, [skill.name]: true }));
    setError(null);
    setStatus(null);
    void (async () => {
      const res = await apiSend(
        withMarketplaceWorkspace(
          `/foxxycode/skills/${encodeURIComponent(skill.name)}/update`,
          workspacePath,
        ),
        "POST",
        undefined,
        sessionId,
      );
      if (!res.ok) {
        setError(res.error || t("settings.skills.updateFailed"));
      } else {
        setStatus(t("settings.skills.updatedStatus", { name: skill.name }));
        await loadInstalled();
        await refreshUpdates();
      }
      setBusy((p) => ({ ...p, [skill.name]: false }));
    })();
  };

  // After a sync of the marketplaces: refresh the list and re-check versions.
  const onSynced = useCallback(async () => {
    invalidateAvailable();
    await loadInstalled();
    await refreshUpdates();
  }, [invalidateAvailable, loadInstalled, refreshUpdates]);

  // Lazily fetch the plugins advertised by configured marketplaces (network /
  // git) when the install control is used and no list is cached; force to
  // refresh after an install. Plain closure over `available` so the "already
  // loaded" guard sees the current value. A request that invalidateAvailable
  // overtook is dropped on arrival.
  const loadAvailable = async (force = false) => {
    if (!force && (available !== null || availableLoading)) return;
    const request = ++availableRequest.current;
    setAvailableLoading(true);
    const items = await fetchAvailable(sessionId, workspacePath);
    if (request !== availableRequest.current) return;
    setAvailable(items);
    setAvailableLoading(false);
  };

  const onInstallPlugin = (p: AvailablePlugin) => {
    setInstallBusy((b) => ({ ...b, [p.name]: true }));
    setError(null);
    setStatus(null);
    void (async () => {
      const res = await apiSend(
        withMarketplaceWorkspace("/foxxycode/skills/install", workspacePath),
        "POST",
        { source: p.source, plugin: p.name },
        sessionId,
      );
      if (!res.ok)
        setError(
          res.error || t("skills.error.install", { name: p.name }),
        );
      else {
        setStatus(t("settings.skills.installedStatus", { name: p.name }));
        // Optimistically drop it from the dropdown right away, then refresh.
        setAvailable((av) => (av ? av.map((a) => (a.name === p.name ? { ...a, installed: true } : a)) : av));
        await loadInstalled();
        await refreshUpdates();
        await loadAvailable(true);
        // Flash the new row (no scroll); it is now installed and usable from
        // the composer's `/` menu straight away (the server drops its slash
        // cache on install).
        setJustInstalled(p.name);
      }
      setInstallBusy((b) => ({ ...b, [p.name]: false }));
    })();
  };

  const installQ = installQuery.trim();
  const { matches: installMatches, more: installMore } =
    filterInstallableMatches(available ?? [], installQ, INSTALL_MENU_LIMIT);
  // The results hang under the box while a search is typed: Escape takes them
  // away, clearing the search, before the drawer hears the key.
  useEscapeCloses(installQ !== "", () => setInstallQuery(""));

  const fieldOverride: FieldOverride = ({ path }) => {
    // Auto-discovery is rendered as its own fieldset at the top of the section
    // (see below); suppress the default inline boolean here.
    if (path === "auto_discovery") {
      return <></>;
    }
    return null;
  };

  const autoDiscoveryOn = value.auto_discovery !== false;
  // Run the backend schema description through the schema RU overlay (same path SchemaForm
  // uses) so this row is localized like every other schema-driven field.
  const autoDiscoveryDesc =
    tSchemaText(
      (schema.properties?.["auto_discovery"] as { description?: string } | undefined)
        ?.description,
    ) || t("settings.skills.autoDiscoveryDesc");

  return (
    <div className="settings-skills-section">
      <fieldset className="settings-fieldset">
        <legend>{t("settings.skills.autoDiscoveryLegend")}</legend>
        <SwitchField
          checked={autoDiscoveryOn}
          onChange={(next) => onChange({ ...value, auto_discovery: next })}
          label={
            autoDiscoveryOn
              ? t("settings.skills.stateEnabled")
              : t("settings.skills.stateDisabled")
          }
          description={autoDiscoveryDesc}
          ariaLabel={t("settings.skills.autoDiscoveryLegend")}
          dataTestId="skills-auto-discovery-toggle"
        />
      </fieldset>

      {/*
        The policy for the project's marketplaces.json stands in a fieldset of
        its own above the list it governs, the way MCP discovery does in the
        MCP tab; unlike that one it is part of the settings document.
      */}
      <SchemaForm
        schema={schema}
        value={value}
        onChange={onChange}
        fieldOverride={fieldOverride}
        groups={[
          {
            id: "skills-marketplace-trust",
            legend: t("skills.trust.legend"),
            description: t("skills.trust.description"),
            paths: ["project_trust"],
          },
        ]}
      />

      <MarketplacesEditor
        activeSessionId={props.activeSessionId}
        workspacePath={workspacePath}
        onSynced={onSynced}
        onChanged={invalidateAvailable}
      />

      <fieldset
        className="settings-fieldset skills-installed-box"
        data-testid="skills-installed"
      >
        {/* How else a skill gets here is about the whole list, so it is the
            (i) of the legend rather than a line inside the box. */}
        <LegendWithHint
          label={t("settings.skills.installedLabel")}
          description={t("skills.install.cliHint")}
        />

        <div className="skills-install">
          <input
            className="settings-input skills-install-input"
            type="text"
            placeholder={t("settings.skills.searchPlaceholder")}
            value={installQuery}
            onChange={(e) => {
              setInstallQuery(e.target.value);
              // After an invalidation the list is fetched again on the next
              // keystroke as well, not only on the next focus.
              void loadAvailable();
            }}
            onFocus={() => void loadAvailable()}
            data-testid="skills-install-input"
          />
          {/* A dropped list hides the menu until the search is used again,
              rather than showing "no matches" for a list nobody fetched. */}
          {installQ && (availableLoading || available !== null) ? (
            <ul className="skills-install-results" data-testid="skills-install-results">
              {available === null ? (
                <li className="skills-install-empty settings-muted">
                  {t("settings.skills.loadingMarketplaces")}
                </li>
              ) : installMatches.length === 0 ? (
                <li className="skills-install-empty settings-muted">
                  {available.length === 0
                    ? t("settings.skills.marketplacesEmpty")
                    : t("settings.skills.noMatches")}
                </li>
              ) : (
                <>
                  {installMatches.map((p) => (
                    <li key={`${p.source}/${p.name}`} className="skills-install-result">
                      <div className="skills-install-result-text">
                        <div className="skills-list-item-name">
                          {p.name}
                          {p.version ? (
                            <span className="skills-list-item-version">v{p.version}</span>
                          ) : null}
                        </div>
                        <div className="skills-list-item-desc">{p.description || p.source}</div>
                      </div>
                      <button
                        type="button"
                        className="settings-btn settings-btn-icon settings-btn-primary"
                        disabled={!!installBusy[p.name]}
                        onClick={() => onInstallPlugin(p)}
                        title={t("settings.skills.install", { name: p.name })}
                        aria-label={t("settings.skills.install", { name: p.name })}
                        data-testid={`skills-install-${p.name}`}
                      >
                        <IconDownload />
                      </button>
                    </li>
                  ))}
                  {installMore > 0 ? (
                    <li
                      className="skills-install-empty settings-muted"
                      data-testid="skills-install-more"
                    >
                      {t("settings.skills.moreResults", { count: installMore })}
                    </li>
                  ) : null}
                </>
              )}
            </ul>
          ) : null}
        </div>

        {error ? <p className="settings-error">{error}</p> : null}
        {status ? <p className="settings-muted">{status}</p> : null}

        {loading ? (
          <p className="settings-muted">{t("settings.loading")}</p>
        ) : (
          <ul className="skills-list">
            {installed.map((sk) => {
              const upd = updates[sk.name];
              const hasUpdate = !!upd?.update_available;
              return (
                <li
                  key={sk.name}
                  className={`skills-list-item${sk.enabled ? "" : " is-disabled"}${sk.name === justInstalled ? " is-just-installed" : ""}`}
                >
                  {/* The on/off switch leads the row, where an icon that said
                      nothing used to stand. */}
                  <button
                    type="button"
                    role="switch"
                    aria-checked={sk.enabled}
                    className="skill-switch"
                    disabled={!!busy[sk.name]}
                    onClick={() => onToggle(sk)}
                    title={
                      sk.enabled
                        ? t("skills.switch.enabledTitle")
                        : t("skills.switch.disabledTitle")
                    }
                    aria-label={t(
                      sk.enabled
                        ? "skills.switch.disableAria"
                        : "skills.switch.enableAria",
                      { name: sk.name },
                    )}
                    data-testid={`skills-toggle-${sk.name}`}
                  >
                    <span className="skill-switch-thumb" />
                  </button>
                  <div className="skills-list-item-text">
                    <div className="skills-list-item-name">
                      {sk.name}
                      {sk.version ? (
                        <span className="skills-list-item-version">
                          v{sk.version}
                        </span>
                      ) : null}
                      {sk.source ? (
                        <span
                          className="skills-list-item-badge"
                          title={t("settings.skills.syncedFrom", {
                            source: sk.source,
                          })}
                        >
                          {t("settings.skills.remoteBadge")}
                        </span>
                      ) : null}
                    </div>
                    {sk.description ? (
                      <div className="skills-list-item-desc">
                        {sk.description}
                      </div>
                    ) : null}
                  </div>
                {hasUpdate ? (
                  <button
                    type="button"
                    className="settings-btn settings-btn-icon settings-btn-primary skills-update-btn"
                    disabled={!!busy[sk.name]}
                    onClick={() => onUpdateSkill(sk)}
                    title={t("settings.skills.updateTitle", {
                      name: sk.name,
                      from: upd?.version || sk.version || "?",
                      to: upd?.latest ?? "",
                    })}
                    aria-label={t("settings.skills.updateAria", {
                      name: sk.name,
                      to: upd?.latest ?? "",
                    })}
                    data-testid={`skills-update-${sk.name}`}
                  >
                    <IconDownload />
                  </button>
                ) : null}
                <button
                  type="button"
                  className="settings-btn settings-btn-icon settings-btn-danger"
                  disabled={!!busy[sk.name] || !!sk.readonly}
                  onClick={() => onRemove(sk)}
                  title={
                    sk.readonly
                      ? t("settings.skills.bundledCannotDelete")
                      : t("settings.skills.delete")
                  }
                  aria-label={t("settings.skills.deleteName", { name: sk.name })}
                  data-testid={`skills-delete-${sk.name}`}
                >
                  <IconTrash />
                </button>
              </li>
            );
          })}
        </ul>
      )}
      </fieldset>
    </div>
  );
}
