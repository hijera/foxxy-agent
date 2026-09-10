import { useEffect, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { setSettingsSectionHash } from "../scheduler/hashRoute";
import { fetchSubagentCatalog, trustSubagent } from "../settings/subagentsApi";

type State =
  | { kind: "checking" }
  | { kind: "idle" }
  | { kind: "needsApproval"; path: string }
  | { kind: "approving" }
  | { kind: "approved" }
  | { kind: "failed"; message: string };

/**
 * Shown under a `spawn_agent` row the runtime refused. It asks the catalog
 * whether that definition is in fact awaiting approval for this workspace, and
 * only then offers the approval - so a spawn that failed for any other reason
 * renders nothing at all.
 *
 * Approving never retries the spawn: the model decides what to do next, and a
 * silent re-run would start work the user only meant to permit.
 */
export function SubagentApprovalNotice(props: {
  agentName: string;
  /** Workspace of this session; the receipt is keyed by it. */
  workspacePath?: string | undefined;
}) {
  const { t } = useT();
  const [state, setState] = useState<State>({ kind: "checking" });
  const { agentName, workspacePath } = props;

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const res = await fetchSubagentCatalog(workspacePath);
      if (cancelled) {
        return;
      }
      if (!res.ok) {
        // The refusal had some other cause, or the catalog is unreachable:
        // either way there is nothing here to offer.
        setState({ kind: "idle" });
        return;
      }
      const entry = res.data.items.find((e) => e.name === agentName);
      setState(
        entry && entry.needs_approval
          ? { kind: "needsApproval", path: entry.path ?? "" }
          : { kind: "idle" },
      );
    })();
    return () => {
      cancelled = true;
    };
  }, [agentName, workspacePath]);

  if (state.kind === "checking" || state.kind === "idle") {
    return null;
  }

  const onApprove = () => {
    setState({ kind: "approving" });
    void (async () => {
      const res = await trustSubagent(agentName, workspacePath);
      setState(res.ok ? { kind: "approved" } : { kind: "failed", message: res.error });
    })();
  };

  return (
    <div className="subagent-approval-notice" data-testid={`subagent-approval-${agentName}`}>
      <div className="subagent-approval-text">
        {state.kind === "approved"
          ? t("messages.subagentApproval.approved", { name: agentName })
          : t("messages.subagentApproval.needed", { name: agentName })}
        {state.kind === "needsApproval" && state.path ? (
          <>
            {" "}
            <code>{state.path}</code>
          </>
        ) : null}
        {state.kind === "failed" ? (
          <span className="settings-error"> {state.message}</span>
        ) : null}
      </div>
      {state.kind === "approved" ? null : (
        <div className="subagent-approval-actions">
          <button
            type="button"
            className="settings-btn settings-btn-approve"
            disabled={state.kind === "approving"}
            onClick={onApprove}
            data-testid={`subagent-approval-approve-${agentName}`}
          >
            {t("messages.subagentApproval.approve")}
          </button>
          <button
            type="button"
            className="settings-btn"
            onClick={() => setSettingsSectionHash("subagents")}
            data-testid={`subagent-approval-settings-${agentName}`}
          >
            {t("messages.subagentApproval.openSettings")}
          </button>
        </div>
      )}
    </div>
  );
}
