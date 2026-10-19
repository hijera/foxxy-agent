import { useCallback, useEffect, useId, useState } from "react";
import { Chevron } from "../components/Chevron";
import { useT } from "../i18n/I18nProvider";
import { SchemaForm, type JsonSchema } from "./SchemaForm";
import {
  pendingApprovalCount,
  scopeBadgeKey,
  shortDigest,
  showsSubagentTrustControl,
  subagentApprovalFacts,
  type SubagentCatalog,
  type SubagentCatalogEntry,
} from "./subagentCatalog";
import { fetchSubagentCatalog, trustSubagent, untrustSubagent } from "./subagentsApi";

// Shield glyph, matching the MCP trust control so the two approvals read as
// the same gesture.
function IconShield() {
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
      <path d="M12 3l7 3v6c0 4.4-3 8.2-7 9-4-.8-7-4.6-7-9V6Z" />
      <path d="M9 12l2 2 4-4" />
    </svg>
  );
}

import { LegendWithHint } from "./FieldHint";

/**
 * The Subagents tab. Hybrid, like the Skills tab: the generated form edits the
 * `subagents` config section (including `project_trust`, which saves with the
 * rest of the document through the footer button), and the catalog below is
 * API-driven — approving a definition writes a receipt immediately, because a
 * receipt is not configuration but a signed statement about one file's current
 * content.
 *
 * A row is the definition's name behind the app's chevron, its badges, its
 * description and its file; the chevron and the name together fold open what
 * the definition declares (model, tools, timeout, ...). Rows carry no rule
 * between them, like every other list of the drawer.
 */
export function SubagentsSection(props: {
  schema: JsonSchema;
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  /**
   * Workspace of the viewed session. Receipts are keyed by workspace and
   * `spawn_agent` decides against the session's own cwd, so the tab has to ask
   * about that workspace rather than the server's. Undefined with no session
   * open: the server then answers for its session default.
   */
  workspacePath?: string | undefined;
}) {
  const { t, tp } = useT();
  const [catalog, setCatalog] = useState<SubagentCatalog | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const factsId = useId();
  const workspacePath = props.workspacePath;

  const load = useCallback(
    async (firstLoad: boolean) => {
      if (firstLoad) {
        setLoading(true);
      }
      const res = await fetchSubagentCatalog(workspacePath);
      if (res.ok) {
        setCatalog(res.data);
        setError(null);
      } else {
        setError(t("settings.subagents.error.load"));
      }
      if (firstLoad) {
        setLoading(false);
      }
    },
    // `t` is stable for a locale; the workspace is what makes this a different
    // question.
    [workspacePath, t],
  );

  useEffect(() => {
    void load(true);
  }, [load]);

  // Same shape as the MCP tab: a refresh never collapses the list height, and
  // one row's failure never blocks the others.
  const withBusy = (key: string, fn: () => Promise<void>) => {
    setBusy((p) => ({ ...p, [key]: true }));
    setError(null);
    void (async () => {
      await fn();
      setBusy((p) => ({ ...p, [key]: false }));
    })();
  };

  const onToggleTrust = (entry: SubagentCatalogEntry) => {
    withBusy(entry.name, async () => {
      const res = entry.trusted
        ? await untrustSubagent(entry.name, workspacePath)
        : await trustSubagent(entry.name, workspacePath);
      if (!res.ok) {
        setError(res.error);
        return;
      }
      await load(false);
    });
  };

  const items = catalog?.items ?? [];
  const policy = catalog?.policy ?? "ask";
  const pending = pendingApprovalCount(items);

  return (
    <div className="settings-subagents-section">
      <SchemaForm
        schema={props.schema}
        value={props.value}
        onChange={props.onChange}
        groups={[
          {
            id: "subagents",
            legend: t("subagents.settings.legend"),
            description: t("subagents.settings.description"),
          },
        ]}
      />

      <fieldset className="settings-fieldset subagents-catalog-box" data-testid="subagents-catalog">
        <LegendWithHint label={t("settings.subagents.legend")} description={<>

          {t("settings.subagents.desc.start")} <code>.foxxycode/agents</code>{" "}
          {t("settings.subagents.desc.and")} <code>.claude/agents</code>{" "}
          {t("settings.subagents.desc.end")}
        </>} />
        <p className="settings-field-desc">{t("settings.subagents.policyAppliesAfterSave")}</p>
        {catalog?.workspace ? (
          <p className="settings-field-desc subagents-workspace">
            {t("settings.subagents.workspace")} <code>{catalog.workspace}</code>
          </p>
        ) : null}
        {error ? <p className="settings-error">{error}</p> : null}
        {pending > 0 ? (
          <p className="settings-field-desc" data-testid="subagents-pending-hint">
            {tp("settings.subagents.pendingHint", pending)}
          </p>
        ) : null}

        {items.length === 0 ? (
          <p className="settings-muted" data-testid="subagents-empty">
            {loading ? t("settings.subagents.loading") : t("settings.subagents.empty")}
          </p>
        ) : (
          <ul className="subagents-list" data-testid="subagents-list">
            {items.map((entry, index) => {
              const isOpen = !!open[entry.name];
              const facts = `${factsId}-${index}`;
              const action = isOpen
                ? t("subagents.catalog.hideDeclared")
                : t("subagents.catalog.showDeclared");
              return (
                <li
                  key={entry.name}
                  className={`subagents-item${isOpen ? " is-open" : ""}`}
                  data-testid={`subagent-row-${entry.name}`}
                >
                  <div className="subagents-item-head">
                    <button
                      type="button"
                      className="subagents-toggle"
                      aria-expanded={isOpen}
                      aria-controls={facts}
                      title={action}
                      data-testid={`subagent-toggle-${entry.name}`}
                      onClick={() =>
                        setOpen((p) => ({ ...p, [entry.name]: !isOpen }))
                      }
                    >
                      <Chevron open={isOpen} />
                      <span className="subagents-item-name">{entry.name}</span>
                    </button>
                    <span className="skills-list-item-badge">
                      {t(scopeBadgeKey(entry.scope))}
                    </span>
                    {entry.hidden ? (
                      <span className="skills-list-item-badge">
                        {t("subagents.badge.hidden")}
                      </span>
                    ) : null}
                    {entry.needs_approval ? (
                      <span
                        className="skills-list-item-badge subagents-badge-pending"
                        title={t("subagents.badge.needsApprovalTitle", {
                          name: entry.name,
                        })}
                        data-testid={`subagent-pending-${entry.name}`}
                      >
                        {t("settings.subagents.badge.needsApproval")}
                      </span>
                    ) : null}
                  </div>
                  <div className="subagents-item-body">
                    {/*
                      Plain text on purpose: this description comes out of a
                      file nobody has approved yet, so it is never markdown and
                      never a link.
                    */}
                    <div className="skills-list-item-desc">
                      {entry.needs_approval
                        ? t("settings.subagents.descriptionWithheld")
                        : entry.description}
                    </div>
                    {entry.path ? (
                      <code
                        className="subagents-file"
                        data-testid={`subagent-file-${entry.name}`}
                      >
                        {entry.path}
                      </code>
                    ) : null}
                    <dl
                      id={facts}
                      className="subagents-facts"
                      data-testid={`subagent-declared-${entry.name}`}
                      hidden={!isOpen}
                    >
                      {subagentApprovalFacts(entry).map((fact) => (
                        <div key={fact.labelKey}>
                          <dt>{t(fact.labelKey)}</dt>
                          <dd>
                            <code>{fact.valueKey ? t(fact.valueKey) : fact.value}</code>
                          </dd>
                        </div>
                      ))}
                    </dl>
                  </div>
                  {showsSubagentTrustControl(entry, policy) ? (
                    <button
                      type="button"
                      className={`settings-btn settings-btn-icon${entry.trusted ? "" : " settings-btn-approve"}`}
                      disabled={!!busy[entry.name]}
                      onClick={() => onToggleTrust(entry)}
                      title={
                        entry.trusted
                          ? t("settings.subagents.trust.approvedTitle", {
                              digest: shortDigest(entry.digest),
                            })
                          : t("settings.subagents.trust.approveTitle", { name: entry.name })
                      }
                      aria-label={t(
                        entry.trusted
                          ? "settings.subagents.trust.withdrawAria"
                          : "settings.subagents.trust.approveAria",
                        { name: entry.name },
                      )}
                      data-testid={`subagent-trust-${entry.name}`}
                    >
                      <IconShield />
                    </button>
                  ) : null}

                {entry.needs_approval ? (
                  <div
                    className="mcp-trust-note"
                    data-testid={`subagent-trust-note-${entry.name}`}
                  >
                    <p>
                      {t("settings.subagents.trust.note.declaredBy")}{" "}
                      <code>{entry.path ?? ""}</code>
                      {t("settings.subagents.trust.note.travels")}
                    </p>
                    <dl className="mcp-trust-facts">
                      {subagentApprovalFacts(entry).map((fact) => (
                        <div key={fact.labelKey}>
                          <dt>{t(fact.labelKey)}</dt>
                          <dd>
                            <code>{fact.valueKey ? t(fact.valueKey) : fact.value}</code>
                          </dd>
                        </div>
                      ))}
                      <div>
                        <dt>{t("settings.subagents.fact.digest")}</dt>
                        <dd>
                          <code title={entry.digest ?? ""}>{shortDigest(entry.digest)}</code>
                        </dd>
                      </div>
                    </dl>
                    <p>{t("settings.subagents.trust.note.upperBound")}</p>
                  </div>
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
