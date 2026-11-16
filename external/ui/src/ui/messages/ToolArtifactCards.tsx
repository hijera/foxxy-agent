import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

import { downloadToolArtifact, type ToolArtifact } from "../chat/toolArtifacts";
import { ApiImage, ApiImageLightbox } from "../components/ApiImage";
import { fileTypeIcon } from "./fileTypeIcon";
import { useT } from "../i18n/I18nProvider";
import { remoteApiRequest } from "../env/remoteEnv";
import { useEscapeCloses } from "../components/useEscapeCloses";
import { copyText } from "../clipboard";

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

const isImage = (artifact: ToolArtifact) =>
  Boolean(artifact.previewUrl) || /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(artifact.name);

async function copy(text: string) {
  if (!await copyText(text)) throw new Error("clipboard copy failed");
}

export function ToolArtifactCards(props: {
  artifacts: readonly ToolArtifact[];
  inline?: boolean;
  onMention?: (path: string) => void;
}) {
  const { t } = useT();
  if (props.artifacts.length === 0) return null;
  return (
    <section className={props.inline ? "inline-artifacts" : "tool-artifacts"} aria-label={t("messages.toolArtifacts")}>
      {props.artifacts.map((artifact) => (
        <ArtifactCard key={artifact.id} artifact={artifact} {...(props.inline !== undefined ? { inline: props.inline } : {})} {...(props.onMention ? { onMention: props.onMention } : {})} />
      ))}
    </section>
  );
}

export function ArtifactCard(props: { artifact: ToolArtifact; inline?: boolean; onMention?: (path: string) => void }) {
  const { t } = useT();
  // Only a failed download makes the card report the artifact as unavailable;
  // a failed reveal or clipboard copy leaves the download itself untouched.
  const [downloadFailed, setDownloadFailed] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [menu, setMenu] = useState(false);
  const [lightbox, setLightbox] = useState(false);
  // A failed action that changes nothing (reveal) reports itself on the card's
  // meta line for a moment instead of being swallowed silently.
  const [notice, setNotice] = useState<string | null>(null);
  const noticeTimer = useRef<number | null>(null);
  const flashNotice = (text: string) => {
    setNotice(text);
    if (noticeTimer.current !== null) window.clearTimeout(noticeTimer.current);
    noticeTimer.current = window.setTimeout(() => setNotice(null), 4000);
  };
  useEffect(() => () => {
    if (noticeTimer.current !== null) window.clearTimeout(noticeTimer.current);
  }, []);
  const menuRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [menuPosition, setMenuPosition] = useState({ left: 0, top: 0 });
  const artifact = props.artifact;
  const image = isImage(artifact);
  const unavailable = !artifact.url || downloadFailed;
  const extension = artifact.name.includes(".") ? artifact.name.split(".").pop()!.toUpperCase() : "FILE";
  const relative = artifact.relativePath || artifact.sourcePath || artifact.name;
  const typeLabel = fileTypeIcon("", artifact.name).label;

  useEscapeCloses(menu, () => {
    setMenu(false);
    triggerRef.current?.focus();
  });

  useLayoutEffect(() => {
    if (!menu || !menuRef.current || !triggerRef.current) return;
    const anchor = triggerRef.current.getBoundingClientRect();
    const bounds = menuRef.current.getBoundingClientRect();
    const left = Math.max(8, Math.min(anchor.right - bounds.width, window.innerWidth - bounds.width - 8));
    const below = anchor.bottom + 4;
    const top = below + bounds.height <= window.innerHeight - 8 ? below : anchor.top - bounds.height - 4;
    setMenuPosition({ left, top: Math.max(8, Math.min(top, window.innerHeight - bounds.height - 8)) });
  }, [menu]);

  useEffect(() => {
    if (!menu) return;
    const close = (event: MouseEvent) => {
      const target = event.target as Node;
      if (!menuRef.current?.contains(target) && !triggerRef.current?.contains(target)) setMenu(false);
    };
    const dismiss = (event: Event) => {
      if (event.target instanceof Node && menuRef.current?.contains(event.target)) return;
      setMenu(false);
    };
    document.addEventListener("mousedown", close);
    window.addEventListener("resize", dismiss);
    document.addEventListener("scroll", dismiss, true);
    return () => {
      document.removeEventListener("mousedown", close);
      window.removeEventListener("resize", dismiss);
      document.removeEventListener("scroll", dismiss, true);
    };
  }, [menu]);

  const reveal = async () => {
    if (!artifact.revealUrl) return;
    const init: RequestInit = { method: "POST" };
    const request = remoteApiRequest(artifact.revealUrl, init);
    const response = await fetch(request?.url || artifact.revealUrl, request?.init ?? init);
    if (!response.ok) throw new Error(`reveal failed (${response.status})`);
  };
  const action = (fn: () => void | Promise<void>, onReject?: () => void) => {
    setMenu(false);
    void Promise.resolve(fn()).catch(() => onReject?.());
  };
  return (
    <article
      className={["tool-artifact-card", props.inline && "inline-artifact-card", image && artifact.previewUrl && "inline-artifact-card--image"].filter(Boolean).join(" ")}
      data-testid={`${props.inline ? "inline" : "tool"}-artifact-card-${artifact.id}`}
      title={artifact.sourcePath || artifact.name}
      onContextMenu={(event) => { event.preventDefault(); setMenu(true); }}
    >
      {image && artifact.previewUrl ? (
        <button type="button" className="inline-artifact-image" onClick={() => setLightbox(true)} aria-label={t("messages.openArtifactImage", { fileName: artifact.name })}>
          <ApiImage className="inline-artifact-thumb" src={artifact.previewUrl} alt="" data-testid="inline-artifact-thumb" />
        </button>
      ) : null}
      <span className="inline-artifact-extension" aria-hidden="true">{extension}</span>
      <span className="tool-artifact-info">
        <span className="tool-artifact-name" title={artifact.name}>{artifact.name}</span>
        <span className={unavailable || notice ? "tool-artifact-meta tool-artifact-meta--error" : "tool-artifact-meta"} {...(notice ? { role: "status" } : {})}>{notice || (unavailable ? t("messages.artifactUnavailable") : props.inline ? formatBytes(artifact.size) : `${typeLabel} · ${formatBytes(artifact.size)}`)}</span>
      </span>
      <button ref={triggerRef} type="button" className="inline-artifact-menu-trigger" aria-label={t("messages.artifactActions", { fileName: artifact.name })} aria-haspopup="menu" aria-expanded={menu} onClick={() => setMenu((open) => !open)}>⋮</button>
      {menu ? createPortal(<div ref={menuRef} className="inline-artifact-menu" role="menu" style={menuPosition}>
        <button role="menuitem" type="button" disabled={!props.onMention || (!artifact.sourcePath && !artifact.relativePath)} onClick={() => action(() => props.onMention?.(relative))}>{t("messages.artifactMention")}</button>
        <button role="menuitem" type="button" onClick={() => action(() => copy(artifact.name), () => flashNotice(t("messages.artifactCopyFailed")))}>{t("messages.artifactCopyName")}</button>
        <button role="menuitem" type="button" onClick={() => action(() => copy(relative), () => flashNotice(t("messages.artifactCopyFailed")))}>{t("messages.artifactCopyRelative")}</button>
        <button role="menuitem" type="button" disabled={!artifact.sourcePath} onClick={() => action(() => copy(artifact.sourcePath!), () => flashNotice(t("messages.artifactCopyFailed")))}>{t("messages.artifactCopyAbsolute")}</button>
        <button role="menuitem" type="button" disabled={unavailable || downloading} onClick={() => action(async () => { setDownloading(true); try { await downloadToolArtifact(artifact); } finally { setDownloading(false); } }, () => setDownloadFailed(true))}>{downloading ? t("messages.artifactDownloading") : t("messages.downloadArtifactButton")}</button>
        <button role="menuitem" type="button" disabled={!artifact.revealUrl} title={!artifact.revealUrl ? t("messages.artifactRevealUnavailable") : undefined} onClick={() => action(reveal, () => flashNotice(t("messages.artifactRevealFailed")))}>{t("messages.artifactReveal")}</button>
      </div>, document.body) : null}
      {lightbox && artifact.previewUrl ? <ApiImageLightbox src={artifact.previewUrl} alt={artifact.name} onClose={() => setLightbox(false)} /> : null}
    </article>
  );
}
