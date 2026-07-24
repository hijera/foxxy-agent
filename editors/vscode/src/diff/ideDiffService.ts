import * as path from "path";
import * as vscode from "vscode";
import { IdeEventClient } from "./ideEventClient";
import { EditEvent, isApplied, isOpenFile, isProposed } from "./editEvent";
import { httpPost } from "../util/http";
import { readSettings } from "../settings";
import { t } from "../i18n/bundle";
import { computeLineFragments, LineFragment } from "./lineFragments";

/** VS Code port of `editors/intellij/.../diff/FoxxyCodeIdeDiffService.kt`.
 *
 *  Subscribes to the foxxycode IDE event stream and renders each agent file edit
 *  in the real editor with green/red line decorations and an Accept/Reject (or
 *  Revert) decision, plus a Show diff action backed by `vscode.diff`. */
export class IdeDiffService {
  private client: IdeEventClient | null = null;
  private clientBase: string | null = null;
  /** Active decoration types per absolute (normalized) path. */
  private readonly decorations = new Map<string, vscode.TextEditorDecorationType>();

  constructor(
    private readonly workspaceRoot: string | undefined,
    private readonly log?: (line: string) => void,
  ) {}

  /** Starts (or, after a server restart on a new port, re-points) the event stream. */
  startIfNeeded(baseUrl: string): void {
    if (this.client && this.clientBase === baseUrl) return;
    this.client?.stop();
    const c = new IdeEventClient(baseUrl, (ev) => this.onEvent(ev));
    this.client = c;
    this.clientBase = baseUrl;
    c.start();
  }

  stop(): void {
    this.client?.stop();
    this.client = null;
    this.clientBase = null;
    this.clearAllDecorations();
  }

  dispose(): void {
    this.stop();
  }

  private onEvent(ev: EditEvent): void {
    // User-initiated open ("Show in IDE"): the plan file lives in the session
    // bundle outside the workspace, and it is not a diff — so it runs before the
    // nativeDiffs / in-project guards below.
    if (isOpenFile(ev)) {
      void this.openFile(ev.path);
      return;
    }
    const s = readSettings();
    if (!s.nativeDiffs) return;
    if (!this.isInProject(ev.path)) return;
    if (isProposed(ev)) {
      if (s.autoApproveEdits) {
        void this.respondPermission(ev, "allow");
        return;
      }
      void this.handleProposed(ev);
    } else if (isApplied(ev)) {
      void this.handleApplied(ev);
    }
  }

  /** Opens a file in the editor area and focuses it (no decorations, no diff). */
  private async openFile(path: string): Promise<void> {
    const target = (path || "").trim();
    if (target === "") return;
    try {
      const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(target));
      await vscode.window.showTextDocument(doc, { preview: false });
    } catch (e) {
      this.log?.(`[foxxycode] open_file failed for ${target}: ${String(e)}`);
    }
  }

  // ---- inline rendering ------------------------------------------------------

  private async handleProposed(ev: EditEvent): Promise<void> {
    const editor = await this.openAndHighlight(ev, false);
    if (editor) this.notifyProposed(ev);
  }

  private async handleApplied(ev: EditEvent): Promise<void> {
    await this.openAndHighlight(ev, true);
    this.notifyApplied(ev);
  }

  /** Opens the file and highlights changed line ranges. */
  private async openAndHighlight(
    ev: EditEvent,
    useAfterRanges: boolean,
  ): Promise<vscode.TextEditor | null> {
    const uri = vscode.Uri.file(ev.path);
    let doc: vscode.TextDocument;
    try {
      doc = await vscode.workspace.openTextDocument(uri);
    } catch {
      return null;
    }
    const editor = await vscode.window.showTextDocument(doc, { preview: false, preserveFocus: true });

    this.clearDecorations(ev.path);

    const fragments = computeLineFragments(ev.before, ev.after, useAfterRanges);
    if (fragments.length === 0) return editor;

    const addedRanges: vscode.Range[] = [];
    const removedRanges: vscode.Range[] = [];
    const lineCount = doc.lineCount;
    for (const f of fragments) {
      if (f.endLine <= f.startLine) continue;
      if (f.startLine >= lineCount) continue;
      const fromLine = Math.max(0, Math.min(f.startLine, lineCount - 1));
      const toLine = Math.max(0, Math.min(f.endLine - 1, lineCount - 1));
      const range = new vscode.Range(fromLine, 0, toLine, doc.lineAt(toLine).text.length);
      if (f.kind === "del") removedRanges.push(range);
      else addedRanges.push(range);
    }

    const addedType = vscode.window.createTextEditorDecorationType({
      backgroundColor: addedDecorationColor(),
      isWholeLine: true,
    });
    const removedType = vscode.window.createTextEditorDecorationType({
      backgroundColor: removedDecorationColor(),
      isWholeLine: true,
    });
    editor.setDecorations(addedType, addedRanges);
    editor.setDecorations(removedType, removedRanges);
    this.decorations.set(this.normalize(ev.path), addedType);
    // Track both types so we dispose them on clear; store the second under a sibling key.
    this.decorations.set(this.normalize(ev.path) + "#del", removedType);
    return editor;
  }

  private clearDecorations(pathArg: string): void {
    const key = this.normalize(pathArg);
    const a = this.decorations.get(key);
    if (a) {
      a.dispose();
      this.decorations.delete(key);
    }
    const d = this.decorations.get(key + "#del");
    if (d) {
      d.dispose();
      this.decorations.delete(key + "#del");
    }
  }

  private clearAllDecorations(): void {
    for (const d of this.decorations.values()) d.dispose();
    this.decorations.clear();
  }

  // ---- notifications / decisions --------------------------------------------

  private notifyProposed(ev: EditEvent): void {
    const fileName = baseName(ev.path);
    void vscode.window
      .showInformationMessage(
        t("diff.notify.proposed.title", fileName),
        t("diff.action.accept"),
        t("diff.action.reject"),
        t("diff.action.showDiff"),
      )
      .then((choice) => {
        if (choice === t("diff.action.accept")) {
          void this.respondPermission(ev, "allow");
          this.clearDecorations(ev.path);
        } else if (choice === t("diff.action.reject")) {
          void this.respondPermission(ev, "reject");
          this.clearDecorations(ev.path);
        } else if (choice === t("diff.action.showDiff")) {
          void this.showDiff(ev);
        }
      });
  }

  private notifyApplied(ev: EditEvent): void {
    const fileName = baseName(ev.path);
    void vscode.window
      .showInformationMessage(
        t("diff.notify.applied.title", fileName),
        t("diff.action.revert"),
        t("diff.action.showDiff"),
      )
      .then((choice) => {
        if (choice === t("diff.action.revert")) {
          void this.revert(ev).then(() => this.clearDecorations(ev.path));
        } else if (choice === t("diff.action.showDiff")) {
          void this.showDiff(ev);
        }
      });
  }

  private async showDiff(ev: EditEvent): Promise<void> {
    const beforeUri = beforeUriFor(ev);
    const afterUri = afterUriFor(ev);
    const title = `${baseName(ev.path)} — ${t("diff.window.before")} ⇄ ${t("diff.window.after")}`;
    await vscode.commands.executeCommand("vscode.diff", beforeUri, afterUri, title);
  }

  /** Native per-edit rollback: restore the file's pre-edit content. */
  private async revert(ev: EditEvent): Promise<void> {
    const uri = vscode.Uri.file(ev.path);
    try {
      const ws = new vscode.WorkspaceEdit();
      ws.replace(uri, fullRange(await vscode.workspace.openTextDocument(uri)), ev.before);
      await vscode.workspace.applyEdit(ws);
    } catch (e) {
      this.log?.(`[foxxycode] revert failed: ${(e as Error).message}`);
    }
  }

  // ---- http -----------------------------------------------------------------

  private async respondPermission(ev: EditEvent, optionId: "allow" | "reject"): Promise<void> {
    if (!ev.sessionId || !ev.toolCallId || !this.clientBase) return;
    const base = this.clientBase.endsWith("/") ? this.clientBase : this.clientBase + "/";
    const url = `${base}foxxycode/sessions/${ev.sessionId}/permission`;
    const body = JSON.stringify({ toolCallId: ev.toolCallId, optionId });
    try {
      await httpPost(url, { body, timeoutMs: 5000 });
    } catch (e) {
      this.log?.(`[foxxycode] permission POST failed: ${(e as Error).message}`);
    }
  }

  // ---- helpers --------------------------------------------------------------

  private isInProject(p: string): boolean {
    if (!this.workspaceRoot) return false;
    return this.normalize(p).startsWith(this.normalize(this.workspaceRoot));
  }

  private normalize(p: string): string {
    const s = p.replace(/\\/g, "/");
    // VS Code paths on Windows and macOS are case-insensitive in practice for our use.
    return process.platform === "win32" ? s.toLowerCase() : s;
  }
}

// ---- module-level helpers ---------------------------------------------------

function addedDecorationColor(): string {
  // Light: subtle green; Dark: subtle dark green — mirrors IntelliJ's JBColor pair.
  return "rgba(38, 162, 105, 0.18)";
}

function removedDecorationColor(): string {
  return "rgba(239, 68, 68, 0.18)";
}

function baseName(p: string): string {
  return p.replace(/\\/g, "/").split("/").pop() ?? p;
}

function fullRange(doc: vscode.TextDocument): vscode.Range {
  const last = doc.lineCount - 1;
  return new vscode.Range(0, 0, Math.max(0, last), doc.lineAt(Math.max(0, last)).text.length);
}

// ---- virtual document providers for the Show diff action --------------------

const BEFORE_SCHEME = "foxxycode-before";
const AFTER_SCHEME = "foxxycode-after";
const beforeRegistry = new Map<string, string>();
const afterRegistry = new Map<string, string>();

let providersRegistered = false;

function ensureProvidersRegistered(): void {
  if (providersRegistered) return;
  providersRegistered = true;
  vscode.workspace.registerTextDocumentContentProvider(BEFORE_SCHEME, {
    provideTextDocumentContent(uri: vscode.Uri): string {
      return beforeRegistry.get(uri.path) ?? "";
    },
  });
  vscode.workspace.registerTextDocumentContentProvider(AFTER_SCHEME, {
    provideTextDocumentContent(uri: vscode.Uri): string {
      return afterRegistry.get(uri.path) ?? "";
    },
  });
}

function beforeUriFor(ev: EditEvent): vscode.Uri {
  ensureProvidersRegistered();
  const key = `${ev.toolCallId}:${ev.path}:before`;
  beforeRegistry.set(key, ev.before);
  return vscode.Uri.from({ scheme: BEFORE_SCHEME, path: key });
}

function afterUriFor(ev: EditEvent): vscode.Uri {
  ensureProvidersRegistered();
  const key = `${ev.toolCallId}:${ev.path}:after`;
  afterRegistry.set(key, ev.after);
  return vscode.Uri.from({ scheme: AFTER_SCHEME, path: key });
}

// re-export for tests
export { baseName, fullRange };
export const _internal = { beforeRegistry, afterRegistry };
export const BEFORE_SCHEME_NAME = BEFORE_SCHEME;
export const AFTER_SCHEME_NAME = AFTER_SCHEME;
