import * as vscode from "vscode";
import { afterUri, beforeUri } from "../diff/virtualDocs";
import { t } from "../i18n/bundle";
import {
  ChangedFile,
  fetchLastSessionId,
  fetchSessionChanges,
  statLabel,
} from "./sessionChanges";

/** VS Code counterpart of the IntelliJ `FoxxyCodeSessionChangesService`.
 *
 *  A tree view rather than a one-shot picker: reviewing a session is a
 *  back-and-forth between files, and a list that stays put while diffs open
 *  beside it is what that needs. Selecting a row opens the native diff editor
 *  over two virtual documents, because neither side is guaranteed to exist on
 *  disk — the "before" of a created file never did, and a rolled-back session
 *  has no "after" at all. */
export class SessionChangesProvider
  implements vscode.TreeDataProvider<ChangedFile>, vscode.Disposable
{
  public static readonly viewId = "foxxycode.changes";

  private readonly emitter = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.emitter.event;

  private files: ChangedFile[] = [];
  private baseUrl: string | null = null;
  private loading = false;

  constructor(private readonly log?: (line: string) => void) {}

  /** Called when the backend starts or moves to a new port. */
  setBaseUrl(baseUrl: string | null): void {
    this.baseUrl = baseUrl;
    void this.refresh();
  }

  dispose(): void {
    this.emitter.dispose();
  }

  getTreeItem(file: ChangedFile): vscode.TreeItem {
    const item = new vscode.TreeItem(
      baseName(file.path),
      vscode.TreeItemCollapsibleState.None,
    );
    item.description = statLabel(file, t("changes.binary"));
    item.tooltip = `${file.path} — ${t(statusKey(file.status))}`;
    item.resourceUri = vscode.Uri.file(file.path);
    item.contextValue = "foxxycodeChangedFile";
    item.command = {
      command: "foxxycode.openChangedFile",
      title: t("changes.openDiff"),
      arguments: [file],
    };
    return item;
  }

  getChildren(): ChangedFile[] {
    return this.files;
  }

  /** Re-reads the change set of the session the panel currently has open. */
  async refresh(): Promise<void> {
    if (this.loading) return;
    const base = this.baseUrl;
    if (!base) {
      this.files = [];
      this.emitter.fire();
      return;
    }
    this.loading = true;
    try {
      const sessionId = await fetchLastSessionId(base);
      this.files = sessionId ? await fetchSessionChanges(base, sessionId) : [];
    } catch (e) {
      // A restarting backend empties the view rather than surfacing an error
      // toast on a background refresh.
      this.log?.(`[foxxycode] session changes refresh failed: ${String(e)}`);
      this.files = [];
    } finally {
      this.loading = false;
      this.emitter.fire();
    }
  }

  /** Opens one changed file in the native diff editor. */
  async openDiff(file: ChangedFile): Promise<void> {
    if (file.binary) {
      void vscode.window.showInformationMessage(t("changes.binaryBody"));
      return;
    }
    const key = `changes:${file.path}`;
    const title = `${baseName(file.path)} — ${t("diff.window.before")} ⇄ ${t("diff.window.after")}`;
    await vscode.commands.executeCommand(
      "vscode.diff",
      beforeUri(`${key}:before`, file.before),
      afterUri(`${key}:after`, file.after),
      title,
    );
  }

  /** Brings the view forward and reloads it (the toolbar button and the
   *  Review action on the SPA card both land here). A row click on the card
   *  names its file in [focusPath]; that file's diff opens right away instead
   *  of making the user find the row again in the tree. */
  async reveal(focusPath?: string): Promise<void> {
    await this.refresh();
    await vscode.commands.executeCommand(`${SessionChangesProvider.viewId}.focus`);
    if (this.files.length === 0) {
      void vscode.window.showInformationMessage(t("changes.none"));
      return;
    }
    if (focusPath) {
      const file = this.files.find((f) => f.path === focusPath);
      if (file) {
        await this.openDiff(file);
      }
    }
  }
}

function baseName(p: string): string {
  return p.replace(/\\/g, "/").split("/").pop() ?? p;
}

function statusKey(status: string): string {
  switch (status) {
    case "added":
      return "changes.status.added";
    case "deleted":
      return "changes.status.deleted";
    default:
      return "changes.status.modified";
  }
}
