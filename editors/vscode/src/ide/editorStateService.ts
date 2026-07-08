import * as vscode from "vscode";
import { httpPost } from "../util/http";
import { readSettings } from "../settings";
import {
  buildEditorStateSnapshot,
  editorStateRequestBody,
  sameSnapshot,
  type EditorStateSnapshot,
} from "./editorStatePayload";

const DEBOUNCE_MS = 300;

/** Reports the set of open editor tabs and the focused file to the foxxycode
 *  backend (POST /foxxycode/ide/editor-state) whenever the editor selection
 *  changes. The backend injects the latest snapshot into each agent turn so the
 *  model knows which files the user is actively viewing — the same "open tabs"
 *  context other coding agents provide.
 *
 *  Mirrors the lifecycle of `diff/ideDiffService.ts`: `startIfNeeded(baseUrl)`
 *  wires the subscriptions and `dispose()` tears them down. Gated by the
 *  `foxxycode.trackOpenFiles` setting (default true). */
export class EditorStateService {
  private baseUrl: string | null = null;
  private readonly subscriptions: vscode.Disposable[] = [];
  private debounce: ReturnType<typeof setTimeout> | null = null;
  private last: EditorStateSnapshot | null = null;

  constructor(private readonly log?: (line: string) => void) {}

  /** Starts (or, after a server restart on a new port, re-points) reporting. */
  startIfNeeded(baseUrl: string): void {
    const rebased = this.baseUrl !== baseUrl;
    this.baseUrl = baseUrl;
    if (this.subscriptions.length === 0) {
      const onChange = (): void => this.schedule();
      this.subscriptions.push(
        vscode.window.onDidChangeActiveTextEditor(onChange),
        vscode.window.onDidChangeVisibleTextEditors(onChange),
        vscode.window.tabGroups.onDidChangeTabs(onChange),
      );
    }
    if (rebased) this.last = null; // force a resend to the new server
    this.schedule();
  }

  dispose(): void {
    if (this.debounce) {
      clearTimeout(this.debounce);
      this.debounce = null;
    }
    for (const d of this.subscriptions) d.dispose();
    this.subscriptions.length = 0;
    this.baseUrl = null;
    this.last = null;
  }

  private schedule(): void {
    if (this.debounce) clearTimeout(this.debounce);
    this.debounce = setTimeout(() => {
      this.debounce = null;
      void this.report();
    }, DEBOUNCE_MS);
  }

  /** Absolute paths of every open tab across all groups that is backed by a
   *  real file on disk (skips untitled buffers, diff views, previews, etc.). */
  private collectOpenFiles(): string[] {
    const out: string[] = [];
    for (const group of vscode.window.tabGroups.all) {
      for (const tab of group.tabs) {
        const input = tab.input as { uri?: vscode.Uri } | undefined;
        const uri = input?.uri;
        if (uri && uri.scheme === "file") out.push(uri.fsPath);
      }
    }
    return out;
  }

  private activeFile(): string | undefined {
    const doc = vscode.window.activeTextEditor?.document;
    if (doc && doc.uri.scheme === "file") return doc.uri.fsPath;
    return undefined;
  }

  private async report(): Promise<void> {
    if (!this.baseUrl) return;
    if (!readSettings().trackOpenFiles) return;

    const snap = buildEditorStateSnapshot(this.collectOpenFiles(), this.activeFile());
    if (this.last && sameSnapshot(this.last, snap)) return;
    this.last = snap;

    const url = `${this.baseUrl.replace(/\/$/, "")}/foxxycode/ide/editor-state`;
    try {
      await httpPost(url, { body: editorStateRequestBody(snap) });
    } catch (e) {
      this.log?.(`[foxxycode] editor-state post failed: ${(e as Error).message ?? String(e)}`);
    }
  }
}
