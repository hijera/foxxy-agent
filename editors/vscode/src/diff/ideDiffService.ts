import * as path from "path";
import * as vscode from "vscode";
import { IdeEventClient } from "./ideEventClient";
import {
  EditEvent,
  isApplied,
  isOpenFile,
  isProposed,
  isRevealFile,
} from "./editEvent";
import { httpPost } from "../util/http";
import { readSettings } from "../settings";
import { t } from "../i18n/bundle";
import { LineFragment } from "./lineFragments";
import { InlineHighlightTracker } from "./inlineHighlights";
import { editorReviewText, moveReviewHunks, reviewHunks, reviewedTextMatches, reviewHunksOverlap, type ReviewHunk } from "./reviewHunks";

interface DecorationPair {
  added: vscode.TextEditorDecorationType;
  removed: vscode.TextEditorDecorationType;
}

interface ReviewEntry {
  offer: ReviewOffer;
  document: vscode.TextDocument;
  version: number;
  hunks: (ReviewHunk & { id: number })[];
}

interface ReviewOffer {
  event: EditEvent;
  installed: boolean;
  pending: boolean;
}

const REVIEW_COMMAND = "foxxycode.resolveInlineEdit";

/** VS Code port of `editors/intellij/.../diff/FoxxyCodeIdeDiffService.kt`.
 *
 *  Subscribes to the foxxycode IDE event stream and renders each agent file edit
 *  in the real editor with line decorations and per-block Accept/Reject actions.
 *  Show diff is available for a separate comparison. */
export class IdeDiffService {
  private client: IdeEventClient | null = null;
  private clientBase: string | null = null;
  /** Active decoration types per absolute (normalized) path. */
  private readonly decorations = new Map<string, DecorationPair>();
  /** Which lines carry the latest edit of each file; see InlineHighlightTracker. */
  private readonly highlights = new InlineHighlightTracker();
  private readonly subscriptions: vscode.Disposable[];
  private readonly lensesChanged = new vscode.EventEmitter<void>();
  private readonly reviews = new Map<string, ReviewEntry>();
  private readonly offers = new Map<string, ReviewOffer>();
  private nextHunk = 0;

  constructor(
    private readonly workspaceRoot: string | undefined,
    private readonly log?: (line: string) => void,
  ) {
    this.subscriptions = [
      this.lensesChanged,
      vscode.workspace.onDidChangeTextDocument((event) => this.onDocumentChanged(event)),
      vscode.workspace.onDidCloseTextDocument((doc) => {
        if (doc.uri.scheme === "file") this.clearDecorations(doc.uri.fsPath);
      }),
      vscode.window.onDidChangeVisibleTextEditors((editors) => {
        for (const editor of editors) {
          if (editor.document.uri.scheme === "file") this.drawReview(editor.document);
        }
      }),
      vscode.languages.registerCodeLensProvider({ scheme: "file" }, {
        onDidChangeCodeLenses: this.lensesChanged.event,
        provideCodeLenses: (doc) => this.reviewLenses(doc),
      }),
      vscode.commands.registerCommand(REVIEW_COMMAND, (uri: vscode.Uri, id: number, reject: boolean) =>
        this.resolveHunk(uri, id, reject)),
    ];
  }

  /** Starts (or, after a server restart on a new port, re-points) the event stream. */
  startIfNeeded(baseUrl: string): void {
    if (this.client && this.clientBase === baseUrl) return;
    this.stop();
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
    for (const subscription of this.subscriptions) subscription.dispose();
  }

  private onEvent(ev: EditEvent): void {
    // User-initiated open ("Show in IDE"): the plan file lives in the session
    // bundle outside the workspace, and it is not a diff — so it runs before the
    // nativeDiffs / in-project guards below.
    if (isOpenFile(ev)) {
      void this.openFile(ev.path);
      return;
    }
    // Exported transcript written to the OS temp dir. Same reasoning as
    // open_file — user-initiated and outside the workspace — but the document is
    // a download, so it goes to the file manager, not an editor tab.
    if (isRevealFile(ev)) {
      void this.revealFile(ev.path);
      return;
    }
    if (ev.type === "open_diff") {
      this.reviewFile(ev, true);
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
      this.reviewFile(ev, false);
    }
  }

  /** Selects an exported document in the OS file manager. */
  private async revealFile(path: string): Promise<void> {
    const target = (path || "").trim();
    if (target === "") return;
    try {
      await vscode.commands.executeCommand(
        "revealFileInOS",
        vscode.Uri.file(target),
      );
    } catch (e) {
      this.log?.(`[foxxycode] reveal_file failed for ${target}: ${String(e)}`);
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

  private reviewFile(ev: EditEvent, explicit: boolean): void {
    void this.openReview(ev, explicit).catch((error) =>
      this.log?.(`[foxxycode] inline review failed: ${String(error)}`));
  }

  private async openReview(ev: EditEvent, explicit: boolean): Promise<void> {
    if (!path.isAbsolute(ev.path)) return;
    const key = this.normalize(ev.path);
    const offer = { event: ev, installed: false, pending: true };
    this.offers.set(key, offer);
    let doc: vscode.TextDocument;
    try {
      doc = await vscode.workspace.openTextDocument(vscode.Uri.file(ev.path));
    } catch {
      if (this.offers.get(key) === offer) {
        this.offers.delete(key);
        if (explicit) await this.showDiff(ev);
        else this.offerComparison(ev);
      }
      return;
    }
    if (this.offers.get(key) !== offer) return;
    this.installReview(doc);
    if (explicit && !offer.installed) {
      this.offers.delete(key);
      await this.showDiff(ev);
      return;
    }
    const editor = await vscode.window.showTextDocument(doc, { preview: false, preserveFocus: !explicit });
    if (this.offers.get(key) !== offer) return;
    this.drawReview(doc);
    if (explicit) {
      const entry = this.currentReview(doc);
      const first = entry?.offer === offer ? entry.hunks[0] : undefined;
      if (first) editor.revealRange(new vscode.Range(doc.positionAt(first.start), doc.positionAt(first.end)),
        vscode.TextEditorRevealType.InCenterIfOutsideViewport);
    }
    this.offerComparison(ev);
  }

  /** The editor's watcher may deliver the write after the SSE event. Inline
   *  actions are installed only on the exact applied version. */
  private installReview(doc: vscode.TextDocument): boolean {
    const key = this.normalize(doc.uri.fsPath);
    const offer = this.offers.get(key);
    if (!offer?.pending) return false;
    const ev = offer.event;
    const eol = doc.eol === vscode.EndOfLine.CRLF ? "\r\n" : "\n";
    const before = editorReviewText(ev.before, eol);
    const after = editorReviewText(ev.after, eol);
    if (before.includes("\uFFFD") || after.includes("\uFFFD")) {
      offer.pending = false;
      return false;
    }
    if (after !== doc.getText()) return false;

    offer.pending = false;
    const added = reviewHunks(before, after).map((hunk) => ({ ...hunk, id: ++this.nextHunk }));
    if (!added.length) return false;
    const existing = ev.type === "open_diff" ? [] : this.currentReview(doc)?.hunks ?? [];
    const retained = existing.filter((hunk) => added.every((next) => !reviewHunksOverlap(hunk, next)));
    offer.installed = true;
    this.highlights.forget(key);
    this.reviews.set(key, { offer, document: doc, version: doc.version, hunks: [...retained, ...added] });
    this.lensesChanged.fire();
    return true;
  }

  private currentReview(doc: vscode.TextDocument): ReviewEntry | undefined {
    const entry = this.reviews.get(this.normalize(doc.uri.fsPath));
    return entry?.document === doc && entry.version === doc.version ? entry : undefined;
  }

  private reviewLenses(doc: vscode.TextDocument): vscode.CodeLens[] {
    return (this.currentReview(doc)?.hunks ?? []).flatMap((hunk) => {
      const pos = doc.positionAt(hunk.start);
      const range = new vscode.Range(pos, pos);
      return [false, true].map((reject) => new vscode.CodeLens(range, {
        title: t(reject ? "diff.action.reject" : "diff.action.accept"),
        command: REVIEW_COMMAND,
        arguments: [doc.uri, hunk.id, reject],
      }));
    });
  }

  private async resolveHunk(uri: vscode.Uri, id: number, reject: boolean): Promise<void> {
    const key = this.normalize(uri.fsPath);
    const editor = vscode.window.visibleTextEditors.find((it) =>
      it.document.uri.scheme === "file" && this.normalize(it.document.uri.fsPath) === key);
    if (!editor) return;
    const doc = editor.document;
    const hunk = this.currentReview(doc)?.hunks.find((it) => it.id === id);
    if (!hunk || !reviewedTextMatches(hunk, doc.getText())) {
      void vscode.window.showWarningMessage(t("diff.review.stale"));
      return;
    }
    if (reject) {
      const range = new vscode.Range(doc.positionAt(hunk.start), doc.positionAt(hunk.end));
      try {
        if (!await editor.edit((edit) => edit.replace(range, hunk.before))) {
          void vscode.window.showWarningMessage(t("diff.review.stale"));
          return;
        }
      } catch (error) {
        this.log?.(`[foxxycode] Reject failed: ${String(error)}`);
        void vscode.window.showWarningMessage(t("diff.review.stale"));
        return;
      }
    }
    const entry = this.reviews.get(key);
    if (entry) {
      entry.hunks = entry.hunks.filter((it) => it.id !== id);
      if (!entry.hunks.length) this.reviews.delete(key);
    }
    this.lensesChanged.fire();
    this.drawReview(doc, true);
  }

  private drawReview(doc: vscode.TextDocument, clear = false): void {
    const entry = this.currentReview(doc);
    if (!entry && !clear) return;
    const fragments = (entry?.hunks ?? []).filter((hunk) => hunk.start < hunk.end).map((hunk): LineFragment => {
      const start = doc.positionAt(hunk.start).line;
      const end = doc.positionAt(Math.max(hunk.start, hunk.end - 1)).line + 1;
      return { startLine: start, endLine: end, kind: "add" };
    });
    this.drawDecorations(this.normalize(doc.uri.fsPath), doc, fragments);
  }

  /** Opens the file and highlights changed line ranges. */
  private async openAndHighlight(
    ev: EditEvent,
    useAfterRanges: boolean,
  ): Promise<vscode.TextEditor | null> {
    // Registered before the first await, so events keep their arrival order
    // even when the handlers of two quick edits resume in the other order.
    const key = this.normalize(ev.path);
    const token = this.highlights.begin(key, { before: ev.before, after: ev.after, useAfterRanges });
    const uri = vscode.Uri.file(ev.path);
    let doc: vscode.TextDocument;
    try {
      doc = await vscode.workspace.openTextDocument(uri);
    } catch {
      return null;
    }
    const editor = await vscode.window.showTextDocument(doc, { preview: false, preserveFocus: true });

    // The document may still hold the text from before the write: the tracker
    // places the lines on what it has now and redraws when it reloads.
    const fragments = this.highlights.place(key, token, doc.getText());
    if (fragments === null) return editor; // a newer edit of this file owns the highlights
    if (!this.currentReview(doc)) this.drawDecorations(key, doc, fragments, editor);
    return editor;
  }

  private onDocumentChanged({ document: doc, contentChanges }: vscode.TextDocumentChangeEvent): void {
    if (doc.uri.scheme !== "file" || !contentChanges.length) return;
    const key = this.normalize(doc.uri.fsPath);
    const entry = this.reviews.get(key);
    if (entry?.document === doc) {
      const changes = contentChanges.map((change) => ({
        start: change.rangeOffset,
        end: change.rangeOffset + change.rangeLength,
        text: change.text,
      }));
      const text = doc.getText();
      entry.hunks = moveReviewHunks(entry.hunks, changes).filter((hunk) => reviewedTextMatches(hunk, text));
      entry.version = doc.version;
      if (!entry.hunks.length) this.reviews.delete(key);
      this.lensesChanged.fire();
    }
    if (this.installReview(doc) || entry) {
      this.drawReview(doc, true);
      return;
    }
    const fragments = this.highlights.documentChanged(key, doc.getText(), doc.isDirty);
    if (fragments !== null) this.drawDecorations(key, doc, fragments);
  }

  private drawDecorations(
    key: string,
    doc: vscode.TextDocument,
    fragments: LineFragment[],
    editor?: vscode.TextEditor,
  ): void {
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

    let pair = this.decorations.get(key);
    if (!pair) {
      pair = {
        added: vscode.window.createTextEditorDecorationType({
          backgroundColor: addedDecorationColor(),
          isWholeLine: true,
        }),
        removed: vscode.window.createTextEditorDecorationType({
          backgroundColor: removedDecorationColor(),
          isWholeLine: true,
        }),
      };
      this.decorations.set(key, pair);
    }
    const editors = new Set(vscode.window.visibleTextEditors.filter((e) => e.document === doc));
    if (editor) editors.add(editor);
    for (const e of editors) {
      e.setDecorations(pair.added, addedRanges);
      e.setDecorations(pair.removed, removedRanges);
    }
  }

  private clearDecorations(pathArg: string): void {
    const key = this.normalize(pathArg);
    this.offers.delete(key);
    this.reviews.delete(key);
    this.lensesChanged.fire();
    this.clearHighlights(key);
  }

  private clearHighlights(key: string): void {
    this.highlights.forget(key);
    const pair = this.decorations.get(key);
    if (pair) {
      pair.added.dispose();
      pair.removed.dispose();
      this.decorations.delete(key);
    }
  }

  private clearAllDecorations(): void {
    this.offers.clear();
    this.reviews.clear();
    this.lensesChanged.fire();
    this.highlights.forgetAll();
    for (const pair of this.decorations.values()) {
      pair.added.dispose();
      pair.removed.dispose();
    }
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
          if (!this.reviews.has(this.normalize(ev.path))) this.clearHighlights(this.normalize(ev.path));
        } else if (choice === t("diff.action.reject")) {
          void this.respondPermission(ev, "reject");
          if (!this.reviews.has(this.normalize(ev.path))) this.clearHighlights(this.normalize(ev.path));
        } else if (choice === t("diff.action.showDiff")) {
          void this.showDiff(ev);
        }
      });
  }

  private offerComparison(ev: EditEvent): void {
    void vscode.window.showInformationMessage(
      t("diff.notify.applied.title", baseName(ev.path)),
      t("diff.action.showDiff"),
    ).then((choice) => {
      if (choice === t("diff.action.showDiff")) void this.showDiff(ev);
    });
  }

  private async showDiff(ev: EditEvent): Promise<void> {
    const beforeUri = beforeUriFor(ev);
    const afterUri = afterUriFor(ev);
    const title = `${baseName(ev.path)} — ${t("diff.window.before")} ⇄ ${t("diff.window.after")}`;
    await vscode.commands.executeCommand("vscode.diff", beforeUri, afterUri, title);
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
    const relative = path.relative(this.normalize(this.workspaceRoot), this.normalize(p));
    return path.isAbsolute(p) && relative !== ".." && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative);
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
export { baseName };
export const _internal = { beforeRegistry, afterRegistry };
export const BEFORE_SCHEME_NAME = BEFORE_SCHEME;
export const AFTER_SCHEME_NAME = AFTER_SCHEME;
