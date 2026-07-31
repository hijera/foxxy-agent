import { useCallback, useEffect, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import type { TranslateParams } from "../i18n/i18n";
import { IconTrash } from "./SchemaForm";
import { Switch } from "./Switch";
import {
  MCP_SERVER_TEMPLATE,
  originLabel,
  parseServerEntryJson,
  serverRowToEntryJson,
  validateMCPServerName,
  type MCPScope,
  type MCPServerRow,
} from "./mcpServerJson";

async function fetchServers(refresh = false): Promise<MCPServerRow[]> {
  const res = await fetch(`/foxxycode/mcp${refresh ? "?refresh=1" : ""}`);
  if (!res.ok) return [];
  const data = (await res.json()) as { items?: MCPServerRow[] };
  return data.items ?? [];
}

async function apiSend(
  path: string,
  method: "POST" | "PUT" | "DELETE",
  body?: unknown,
): Promise<{ ok: boolean; error?: string }> {
  const init: RequestInit = { method };
  if (body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(body);
  }
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

// Plug glyph shared with the Skills list style.
function IconServer() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <rect x="2" y="3" width="20" height="7" rx="2" />
      <rect x="2" y="14" width="20" height="7" rx="2" />
      <line x1="6" y1="6.5" x2="6.01" y2="6.5" />
      <line x1="6" y1="17.5" x2="6.01" y2="17.5" />
    </svg>
  );
}

function IconSync() {
  return (
    <svg
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.9"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M21 2v6h-6" />
      <path d="M3 12a9 9 0 0 1 15-6.7L21 8" />
      <path d="M3 22v-6h6" />
      <path d="M21 12a9 9 0 0 1-15 6.7L3 16" />
    </svg>
  );
}

function IconPencil() {
  return (
    <svg
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.9"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z" />
    </svg>
  );
}

function IconChevron(props: { open: boolean }) {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      style={{ transform: props.open ? "rotate(90deg)" : undefined }}
    >
      <polyline points="9 18 15 12 9 6" />
    </svg>
  );
}

type Translate = (key: string, params?: TranslateParams) => string;

function statusTitle(row: MCPServerRow, t: Translate): string {
  switch (row.status) {
    case "connected":
      return t("settings.mcp.status.connected", { count: row.tools.length });
    case "error":
      return row.error || t("settings.mcp.status.probeFailed");
    case "disabled":
      return t("settings.mcp.status.disabled");
    default:
      return row.error || t("settings.mcp.status.unsupported");
  }
}

/** Editor state: null = closed; name empty = adding a new server. */
type EditorState = {
  name: string;
  text: string;
  isNew: boolean;
  scope: MCPScope;
};

/**
 * MCPSection is the Settings -> MCP servers tab: the merged server list from
 * config.yaml, the global ~/.foxxycode/mcp.json, and the local ./.foxxycode/mcp.json
 * in the Cursor style — status dot, scope badge, server switch, expandable
 * per-tool switches, and a JSON editor for mcp.json entries of either scope.
 * All actions talk to /foxxycode/mcp* directly; nothing here touches the
 * settings document.
 */
export function MCPSection() {
  const { t } = useT();
  const [servers, setServers] = useState<MCPServerRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [editorError, setEditorError] = useState<string | null>(null);
  const [editorBusy, setEditorBusy] = useState(false);

  // firstLoad guards the "Loading…" placeholder so refreshes never collapse
  // the list height (same pattern as the Skills tab).
  const loadServers = useCallback(
    async (firstLoad = false, refresh = false) => {
      if (firstLoad) setLoading(true);
      if (refresh) setRefreshing(true);
      setServers(await fetchServers(refresh));
      if (firstLoad) setLoading(false);
      if (refresh) setRefreshing(false);
    },
    [],
  );

  useEffect(() => {
    void loadServers(true);
  }, [loadServers]);

  const withBusy = (key: string, fn: () => Promise<void>) => {
    setBusy((p) => ({ ...p, [key]: true }));
    setError(null);
    void (async () => {
      await fn();
      setBusy((p) => ({ ...p, [key]: false }));
    })();
  };

  const onToggleServer = (row: MCPServerRow) => {
    withBusy(row.name, async () => {
      const action = row.enabled ? "disable" : "enable";
      const res = await apiSend(
        `/foxxycode/mcp/${encodeURIComponent(row.name)}/${action}`,
        "POST",
      );
      if (!res.ok) {
        setError(
          res.error ||
            t(
              row.enabled
                ? "settings.mcp.error.disableServer"
                : "settings.mcp.error.enableServer",
              { name: row.name },
            ),
        );
      } else await loadServers();
    });
  };

  const onToggleTool = (row: MCPServerRow, tool: string, enabled: boolean) => {
    withBusy(`${row.name}__${tool}`, async () => {
      const action = enabled ? "disable" : "enable";
      const res = await apiSend(
        `/foxxycode/mcp/${encodeURIComponent(row.name)}/tools/${encodeURIComponent(tool)}/${action}`,
        "POST",
      );
      if (!res.ok) {
        setError(
          res.error ||
            t(
              enabled
                ? "settings.mcp.error.disableTool"
                : "settings.mcp.error.enableTool",
              { tool },
            ),
        );
      } else await loadServers();
    });
  };

  const onDelete = (row: MCPServerRow) => {
    withBusy(row.name, async () => {
      const res = await apiSend(
        `/foxxycode/mcp/${encodeURIComponent(row.name)}`,
        "DELETE",
      );
      if (!res.ok) {
        setError(
          res.error || t("settings.mcp.error.deleteServer", { name: row.name }),
        );
      } else await loadServers();
    });
  };

  const openAdd = () => {
    setEditorError(null);
    setEditor({
      name: "",
      text: MCP_SERVER_TEMPLATE,
      isNew: true,
      scope: "local",
    });
  };

  const openEdit = (row: MCPServerRow) => {
    setEditorError(null);
    // Editing writes back to the file that owns the row: origin home lives in
    // the global ~/.foxxycode/mcp.json, project in the local ./.foxxycode/mcp.json.
    setEditor({
      name: row.name,
      text: serverRowToEntryJson(row),
      isNew: false,
      scope: row.origin === "home" ? "global" : "local",
    });
  };

  const onEditorSave = () => {
    if (!editor) return;
    const nameErr = validateMCPServerName(editor.name);
    if (nameErr) {
      setEditorError(t(nameErr));
      return;
    }
    const { entry, error: parseErr } = parseServerEntryJson(editor.text);
    if (parseErr || !entry) {
      setEditorError(
        parseErr ? t(parseErr) : t("settings.mcp.validation.invalidEntry"),
      );
      return;
    }
    setEditorBusy(true);
    setEditorError(null);
    void (async () => {
      const res = await apiSend(
        `/foxxycode/mcp/${encodeURIComponent(editor.name.trim())}?scope=${editor.scope}`,
        "PUT",
        entry,
      );
      if (!res.ok) {
        setEditorError(res.error || t("settings.mcp.error.saveServer"));
      } else {
        setEditor(null);
        await loadServers();
      }
      setEditorBusy(false);
    })();
  };

  return (
    <div className="settings-mcp-section">
      <fieldset className="settings-fieldset mcp-servers-box">
        <legend>{t("settings.mcp.legend")}</legend>
        <p className="settings-field-desc">
          {t("settings.mcp.description.start")} <code>config.yaml</code> (
          <code>mcp_servers</code>), {t("settings.mcp.description.global")}{" "}
          <code>~/.foxxycode/mcp.json</code>,{" "}
          {t("settings.mcp.description.local")}{" "}
          <code>./.foxxycode/mcp.json</code> {t("settings.mcp.description.end")}
        </p>

        <div className="mcp-toolbar">
          <button
            type="button"
            className="settings-btn"
            onClick={openAdd}
            data-testid="mcp-add-server"
          >
            {t("settings.mcp.addServer")}
          </button>
          <button
            type="button"
            className="settings-btn settings-btn-icon"
            disabled={refreshing}
            onClick={() => void loadServers(false, true)}
            title={t("settings.mcp.refreshTitle")}
            aria-label={t("settings.mcp.refreshAriaLabel")}
            data-testid="mcp-refresh"
          >
            <IconSync />
          </button>
        </div>

        {error ? <p className="settings-error">{error}</p> : null}

        {editor && editor.isNew ? (
          <MCPEditorCard
            editor={editor}
            error={editorError}
            busy={editorBusy}
            onChange={setEditor}
            onSave={onEditorSave}
            onCancel={() => setEditor(null)}
          />
        ) : null}

        {servers.length === 0 ? (
          loading ? (
            <p className="settings-muted">{t("settings.mcp.loading")}</p>
          ) : (
            <p className="settings-muted">
              {t("settings.mcp.empty.start")} <code>./.foxxycode/mcp.json</code>{" "}
              {t("settings.mcp.empty.orGlobal")}{" "}
              <code>~/.foxxycode/mcp.json</code>;{" "}
              {t("settings.mcp.empty.orConfig")} <code>mcp_servers</code>{" "}
              {t("settings.mcp.empty.inConfig")} <code>config.yaml</code>.
            </p>
          )
        ) : (
          <ul className="mcp-list" data-testid="mcp-list">
            {servers.map((row) => {
              const isOpen = !!expanded[row.name];
              const editable = !row.readonly;
              return (
                <li
                  key={row.name}
                  className={`mcp-list-item${row.enabled ? "" : " is-disabled"}`}
                >
                  <div className="mcp-list-item-head">
                    <button
                      type="button"
                      className="mcp-expand-btn"
                      onClick={() =>
                        setExpanded((p) => ({ ...p, [row.name]: !isOpen }))
                      }
                      title={
                        isOpen
                          ? t("settings.mcp.collapseTools")
                          : t("settings.mcp.expandTools")
                      }
                      aria-label={t(
                        isOpen
                          ? "settings.mcp.collapseToolsAriaLabel"
                          : "settings.mcp.expandToolsAriaLabel",
                        { name: row.name },
                      )}
                      aria-expanded={isOpen}
                      data-testid={`mcp-expand-${row.name}`}
                    >
                      <IconChevron open={isOpen} />
                    </button>
                    <span
                      className={`mcp-status-dot is-${row.status}`}
                      title={statusTitle(row, t)}
                      data-testid={`mcp-status-${row.name}`}
                    />
                    <IconServer />
                    <div className="mcp-list-item-text">
                      <div className="skills-list-item-name">
                        {row.name}
                        <span
                          className="skills-list-item-badge"
                          title={t("settings.mcp.definedIn", {
                            origin: originLabel(row.origin),
                          })}
                        >
                          {t(
                            row.source === "local"
                              ? "settings.mcp.scope.local"
                              : "settings.mcp.scope.global",
                          )}
                        </span>
                        {row.transport !== "stdio" ? (
                          <span className="skills-list-item-badge mcp-badge-transport">
                            {row.transport}
                          </span>
                        ) : null}
                      </div>
                      <div className="skills-list-item-desc mcp-command">
                        {row.status === "error" || row.status === "unsupported"
                          ? row.error
                          : row.command
                            ? [row.command, ...(row.args ?? [])].join(" ")
                            : row.url}
                      </div>
                    </div>
                    <Switch
                      checked={row.enabled}
                      disabled={!!busy[row.name]}
                      onChange={() => onToggleServer(row)}
                      title={
                        row.enabled
                          ? t("settings.mcp.enabledClickToDisable")
                          : t("settings.mcp.disabledClickToEnable")
                      }
                      ariaLabel={t(
                        row.enabled
                          ? "settings.mcp.disableServerAriaLabel"
                          : "settings.mcp.enableServerAriaLabel",
                        { name: row.name },
                      )}
                      dataTestId={`mcp-toggle-${row.name}`}
                    />
                    <button
                      type="button"
                      className="settings-btn settings-btn-icon"
                      disabled={!editable}
                      onClick={() => openEdit(row)}
                      title={
                        editable
                          ? t("settings.mcp.editEntry", {
                              origin: originLabel(row.origin),
                            })
                          : t("settings.mcp.editConfigHint")
                      }
                      aria-label={t("settings.mcp.editAriaLabel", {
                        name: row.name,
                      })}
                      data-testid={`mcp-edit-${row.name}`}
                    >
                      <IconPencil />
                    </button>
                    <button
                      type="button"
                      className="settings-btn settings-btn-icon settings-btn-danger"
                      disabled={!editable || !!busy[row.name]}
                      onClick={() => onDelete(row)}
                      title={
                        editable
                          ? t("settings.mcp.deleteFrom", {
                              origin: originLabel(row.origin),
                            })
                          : t("settings.mcp.deleteConfigHint")
                      }
                      aria-label={t("settings.mcp.deleteAriaLabel", {
                        name: row.name,
                      })}
                      data-testid={`mcp-delete-${row.name}`}
                    >
                      <IconTrash />
                    </button>
                  </div>

                  {editor && !editor.isNew && editor.name === row.name ? (
                    <MCPEditorCard
                      editor={editor}
                      error={editorError}
                      busy={editorBusy}
                      onChange={setEditor}
                      onSave={onEditorSave}
                      onCancel={() => setEditor(null)}
                    />
                  ) : null}

                  {isOpen ? (
                    row.tools.length === 0 ? (
                      <p className="settings-muted mcp-tools-empty">
                        {row.status === "connected"
                          ? t("settings.mcp.noTools")
                          : t("settings.mcp.noToolsUnreachable")}
                      </p>
                    ) : (
                      <ul
                        className="mcp-tools"
                        data-testid={`mcp-tools-${row.name}`}
                      >
                        {row.tools.map((tool) => (
                          <li
                            key={tool.name}
                            className={`mcp-tool-row${tool.enabled ? "" : " is-disabled"}`}
                          >
                            <div className="mcp-tool-text">
                              <div className="mcp-tool-name">{tool.name}</div>
                              {tool.description ? (
                                <div className="skills-list-item-desc">
                                  {tool.description}
                                </div>
                              ) : null}
                            </div>
                            <Switch
                              checked={tool.enabled}
                              disabled={
                                !!busy[`${row.name}__${tool.name}`] ||
                                !row.enabled
                              }
                              onChange={() =>
                                onToggleTool(row, tool.name, tool.enabled)
                              }
                              title={
                                !row.enabled
                                  ? t("settings.mcp.serverDisabled")
                                  : tool.enabled
                                    ? t("settings.mcp.enabledClickToDisable")
                                    : t("settings.mcp.disabledClickToEnable")
                              }
                              ariaLabel={t(
                                tool.enabled
                                  ? "settings.mcp.disableToolAriaLabel"
                                  : "settings.mcp.enableToolAriaLabel",
                                { tool: tool.name, server: row.name },
                              )}
                              dataTestId={`mcp-tool-toggle-${row.name}-${tool.name}`}
                            />
                          </li>
                        ))}
                      </ul>
                    )
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </fieldset>
    </div>
  );
}

/** Cursor-style JSON editor card for one .foxxycode/mcp.json entry. */
function MCPEditorCard(props: {
  editor: EditorState;
  error: string | null;
  busy: boolean;
  onChange: (next: EditorState) => void;
  onSave: () => void;
  onCancel: () => void;
}) {
  const { t } = useT();
  const { editor, error, busy, onChange, onSave, onCancel } = props;
  return (
    <div className="mcp-editor" data-testid="mcp-editor">
      {editor.isNew ? (
        <input
          className="settings-input"
          type="text"
          placeholder={t("settings.mcp.editor.namePlaceholder")}
          value={editor.name}
          onChange={(e) => onChange({ ...editor, name: e.target.value })}
          aria-label={t("settings.mcp.editor.nameAriaLabel")}
          data-testid="mcp-editor-name"
        />
      ) : null}
      {editor.isNew ? (
        <div
          className="mcp-editor-scope"
          role="radiogroup"
          aria-label={t("settings.mcp.editor.scopeAriaLabel")}
        >
          <label className="mcp-editor-scope-option">
            <input
              type="radio"
              name="mcp-editor-scope"
              checked={editor.scope === "local"}
              onChange={() => onChange({ ...editor, scope: "local" })}
              data-testid="mcp-editor-scope-local"
            />
            <span>
              {t("settings.mcp.editor.scopeLocal")} —{" "}
              <code>./.foxxycode/mcp.json</code>
            </span>
          </label>
          <label className="mcp-editor-scope-option">
            <input
              type="radio"
              name="mcp-editor-scope"
              checked={editor.scope === "global"}
              onChange={() => onChange({ ...editor, scope: "global" })}
              data-testid="mcp-editor-scope-global"
            />
            <span>
              {t("settings.mcp.editor.scopeGlobal")} —{" "}
              <code>~/.foxxycode/mcp.json</code>
            </span>
          </label>
        </div>
      ) : null}
      <textarea
        className="settings-input mcp-editor-json"
        rows={8}
        spellCheck={false}
        value={editor.text}
        onChange={(e) => onChange({ ...editor, text: e.target.value })}
        aria-label={t("settings.mcp.editor.jsonAriaLabel")}
        data-testid="mcp-editor-json"
      />
      <p className="settings-field-desc">
        {t("settings.mcp.editor.descriptionStart")} <code>mcpServers</code>{" "}
        {t("settings.mcp.editor.descriptionEntry")} <code>command</code>/
        <code>args</code>/<code>env</code>{" "}
        {t("settings.mcp.editor.descriptionOptions")} <code>disabled</code>{" "}
        {t("settings.mcp.editor.descriptionAnd")} <code>disabledTools</code>.{" "}
        {t("settings.mcp.editor.savedTo")}{" "}
        <code>
          {editor.scope === "global"
            ? "~/.foxxycode/mcp.json"
            : "./.foxxycode/mcp.json"}
        </code>
        .
      </p>
      {error ? <p className="settings-error">{error}</p> : null}
      <div className="mcp-editor-actions">
        <button
          type="button"
          className="settings-btn settings-btn-primary"
          disabled={busy}
          onClick={onSave}
          data-testid="mcp-editor-save"
        >
          {t("settings.mcp.editor.save")}
        </button>
        <button
          type="button"
          className="settings-btn"
          disabled={busy}
          onClick={onCancel}
        >
          {t("settings.mcp.editor.cancel")}
        </button>
      </div>
    </div>
  );
}
