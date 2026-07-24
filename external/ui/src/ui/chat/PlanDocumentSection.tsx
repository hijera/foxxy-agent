import { useCallback, useEffect, useRef, useState } from "react";

import { isEditorEmbed } from "../embedShell";
import { useT } from "../i18n/I18nProvider";
import { Markdown } from "../markdown/Markdown";
import { MarkdownLineEditor } from "../markdown/MarkdownLineEditor";
import { planEditorBody } from "./planContent";

type PlanBodyView = "markdown" | "preview";

const HDR = "X-FoxxyCode-Session-ID";

export type PlanDocumentSectionProps = {
  sessionId: string;
  slug: string;
  name: string;
  overview: string;
  content: string;
  body?: string;
  path?: string;
  discarded?: boolean;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  onDiscard: () => void;
  onRunPlan: () => void;
};

function descriptionLine(overview: string, body: string): string {
  const o = overview.trim();
  if (o) return o;
  const first = body.split("\n").find((l) => l.trim().length > 0);
  return first?.trim() || "";
}

function planFilePath(slug: string, path?: string): string {
  const p = (path ?? "").trim();
  if (p) return p;
  const s = slug.trim();
  return s ? `plans/${s}.plan.md` : "";
}

function PlanPreviewEyeToggle(p: {
  previewOn: boolean;
  onToggle: () => void;
  t: (key: string) => string;
}) {
  return (
    <button
      type="button"
      className={
        p.previewOn ? "plan-document-eye is-on" : "plan-document-eye"
      }
      title={p.t("prompts.planTogglePreview")}
      aria-label={p.t("prompts.planTogglePreview")}
      aria-pressed={p.previewOn}
      data-test="plan_document_preview_toggle"
      onClick={() => p.onToggle()}
    >
      <svg
        className="plan-document-eye-svg"
        viewBox="0 0 24 24"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden
      >
        <path
          d="M2.25 12s3.75-6.75 9.75-6.75S21.75 12 21.75 12s-3.75 6.75-9.75 6.75S2.25 12 2.25 12Z"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="1.6" />
        {!p.previewOn ? (
          <path
            d="M4 4l16 16"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
          />
        ) : null}
      </svg>
    </button>
  );
}

/**
 * Opens the plan file in the host IDE editor. Icon-only and parked next to the
 * preview eye: the footer is too tight for a worded button in a plugin panel, and
 * a document glyph reads faster than "Показать в IDE" wrapped over two lines.
 */
function PlanOpenInIdeButton(p: {
  discarded: boolean;
  onOpenInIde: () => void;
  t: (key: string) => string;
}) {
  const label = p.t("prompts.planOpenInIde");
  return (
    <button
      type="button"
      className="plan-document-ide"
      title={label}
      aria-label={label}
      data-test="plan_document_open_in_ide"
      data-testid="plan_document_open_in_ide"
      disabled={p.discarded}
      onClick={() => p.onOpenInIde()}
    >
      <svg
        className="plan-document-ide-svg"
        viewBox="0 0 24 24"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden
      >
        <path
          d="M14.25 3.75H7.5A1.5 1.5 0 0 0 6 5.25v13.5a1.5 1.5 0 0 0 1.5 1.5h9a1.5 1.5 0 0 0 1.5-1.5V7.5l-3.75-3.75Z"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M14.25 3.75V7.5H18"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M9 12.75h6M9 15.75h4"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
        />
      </svg>
    </button>
  );
}

function PlanDocumentActions(p: {
  discarded: boolean;
  onRunPlan: () => void;
  onDiscard: () => void;
  t: (key: string) => string;
}) {
  return (
    <>
      <button
        type="button"
        className="plan-document-discard"
        data-test="plan_document_discard"
        disabled={p.discarded}
        onClick={() => p.onDiscard()}
      >
        {p.t("prompts.planDiscard")}
      </button>
      <button
        type="button"
        className="plan-document-run"
        data-test="plan_document_run"
        disabled={p.discarded}
        onClick={() => p.onRunPlan()}
      >
        <span className="plan-document-run-ic" aria-hidden>
          ▶
        </span>
        {p.t("prompts.planRun")}
      </button>
    </>
  );
}

export function PlanDocumentSection(props: PlanDocumentSectionProps) {
  const { t } = useT();
  const editorSeed = planEditorBody(props.content, props.body);
  const [draft, setDraft] = useState(editorSeed);
  const [bodyView, setBodyView] = useState<PlanBodyView>("preview");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Local edits not yet accepted by the server: a debounce is pending or a PUT is
  // in flight. While that holds, an incoming snapshot must not overwrite the draft.
  const dirtyRef = useRef(false);
  const discarded = props.discarded === true;
  const previewOn = bodyView === "preview";

  useEffect(() => {
    // Transcript rebuilds re-render this card with the persisted body (which can
    // also come from another window editing the same plan). Reseeding then would
    // throw away keystrokes that have not been saved yet.
    if (dirtyRef.current) return;
    setDraft(planEditorBody(props.content, props.body));
  }, [props.content, props.body]);

  const persist = useCallback(
    async (text: string) => {
      const sid = props.sessionId.trim();
      if (!sid || discarded) return;
      setSaving(true);
      setSaveError("");
      try {
        const res = await fetch(
          `/foxxycode/sessions/${encodeURIComponent(sid)}/plans/${encodeURIComponent(props.slug)}`,
          {
            method: "PUT",
            headers: {
              "Content-Type": "application/json",
              [HDR]: sid,
            },
            body: JSON.stringify({
              body: text,
              ...(props.content.trim() ? { content: props.content } : {}),
            }),
          },
        );
        if (!res.ok) {
          throw new Error(t("prompts.planSaveFailed", { status: res.status }));
        }
        dirtyRef.current = false;
      } catch (e) {
        setSaveError(
          e instanceof Error
            ? e.message
            : t("prompts.planSaveFailed", { status: "" }),
        );
      } finally {
        setSaving(false);
      }
    },
    [props.sessionId, props.slug, props.content, discarded, t],
  );

  // "Show in IDE": the server resolves the plan path from the session bundle and
  // pushes an open_file event to the plugin over /foxxycode/ide/events.
  const openInIde = useCallback(() => {
    const sid = props.sessionId.trim();
    if (!sid || discarded) return;
    void (async () => {
      setSaveError("");
      try {
        const res = await fetch(
          `/foxxycode/sessions/${encodeURIComponent(sid)}/plans/${encodeURIComponent(props.slug)}/open-in-ide`,
          { method: "POST", headers: { [HDR]: sid } },
        );
        if (!res.ok) {
          throw new Error(String(res.status));
        }
      } catch {
        setSaveError(t("prompts.planOpenInIdeFailed"));
      }
    })();
  }, [props.sessionId, props.slug, discarded, t]);

  const scheduleSave = useCallback(
    (text: string) => {
      if (discarded) return;
      dirtyRef.current = true;
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => {
        void persist(text);
      }, 600);
    },
    [persist, discarded],
  );

  const title = props.name.trim() || props.slug;
  const description = descriptionLine(props.overview, editorSeed);
  const filePath = planFilePath(props.slug, props.path);

  // The pane rail only exists in the expanded body, so a collapsed card renders the
  // same icon in its header rather than losing the action entirely.
  const ideButtonVisible = isEditorEmbed();
  const collapsedIdeButton = ideButtonVisible && !props.expanded;

  const cardClass = [
    "plan-document-card",
    props.expanded ? "plan-document-card--expanded" : "",
    discarded ? "plan-document-card--discarded" : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <section
      className="plan-document-frame"
      data-test={
        props.expanded ? "plan_document_section" : "plan_document_collapsed"
      }
    >
      <div className={cardClass}>
        <header
          className={
            collapsedIdeButton
              ? "plan-document-head plan-document-head--with-ide"
              : "plan-document-head"
          }
        >
          <button
            type="button"
            className="plan-document-head-btn"
            onClick={() => props.onExpandedChange(!props.expanded)}
            aria-expanded={props.expanded}
          >
            <span className="plan-document-title" title={filePath}>
              {title}
            </span>
            {!props.expanded && description ? (
              <span className="plan-document-desc">{description}</span>
            ) : null}
          </button>
          {/* Collapsed cards have no body, so the icon rides in the header instead
              of the pane rail — the action stays reachable without expanding. */}
          {collapsedIdeButton ? (
            <div className="plan-document-head-tools">
              <PlanOpenInIdeButton
                discarded={discarded}
                onOpenInIde={openInIde}
                t={t}
              />
            </div>
          ) : null}
          {/* A failure stays visible after the card is collapsed, so an autosave or
              open-in-IDE error is not silently lost when the body is hidden. */}
          {(props.expanded && saving) || saveError ? (
            <div className="plan-document-head-status">
              {saving ? (
                <span className="plan-document-save-hint">{t("prompts.planSaving")}</span>
              ) : null}
              {saveError ? (
                <span className="plan-document-save-error">{saveError}</span>
              ) : null}
            </div>
          ) : null}
        </header>

        {props.expanded ? (
          <div className="plan-document-body">
            <div className="plan-document-pane">
              <div className="plan-document-pane-tools">
                {/* Only inside an editor plugin: in the browser there is no IDE to open. */}
                {ideButtonVisible ? (
                  <PlanOpenInIdeButton
                    discarded={discarded}
                    onOpenInIde={openInIde}
                    t={t}
                  />
                ) : null}
                <PlanPreviewEyeToggle
                  previewOn={previewOn}
                  t={t}
                  onToggle={() =>
                    setBodyView((v) => (v === "preview" ? "markdown" : "preview"))
                  }
                />
              </div>
              <div
                className={
                  previewOn
                    ? "plan-document-pane-inner plan-document-pane-inner--preview"
                    : "plan-document-pane-inner plan-document-pane-inner--markdown"
                }
              >
                {previewOn ? (
                  <div
                    className="plan-document-preview-pane"
                    data-test="plan_document_preview_pane"
                  >
                    {draft.trim() ? (
                      <Markdown text={draft} />
                    ) : (
                      <p className="plan-document-preview-empty">
                        {t("prompts.planPreviewEmpty")}
                      </p>
                    )}
                  </div>
                ) : (
                  <MarkdownLineEditor
                    className="md-line-editor--plan"
                    value={draft}
                    readOnly={discarded}
                    minRows={4}
                    spellCheck
                    gutterTestId="plan_editor_gutter"
                    rootTestId="plan_markdown_editor"
                    aria-label={t("prompts.planBodyAriaLabel")}
                    placeholder={t("prompts.planBodyPlaceholder")}
                    onChange={(v) => {
                      setDraft(v);
                      scheduleSave(v);
                    }}
                  />
                )}
              </div>
            </div>
          </div>
        ) : null}

        <footer className="plan-document-foot">
          <PlanDocumentActions
            discarded={discarded}
            onRunPlan={props.onRunPlan}
            onDiscard={props.onDiscard}
            t={t}
          />
        </footer>
      </div>
    </section>
  );
}
