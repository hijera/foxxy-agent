import * as vscode from "vscode";

/** Virtual document schemes backing every `vscode.diff` this extension opens.
 *
 *  Both sides of an agent diff are content the extension holds in memory: the
 *  "before" side may no longer exist on disk, and for a session change set
 *  neither side does. A text document content provider is how VS Code accepts
 *  that, and one pair of schemes serves every caller so a diff opened from the
 *  inline edit flow and one opened from the Changes view behave identically. */
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

/** URI serving `content` as the left-hand side of a diff, keyed by `key`. */
export function beforeUri(key: string, content: string): vscode.Uri {
  ensureProvidersRegistered();
  beforeRegistry.set(key, content);
  return vscode.Uri.from({ scheme: BEFORE_SCHEME, path: key });
}

/** URI serving `content` as the right-hand side of a diff, keyed by `key`. */
export function afterUri(key: string, content: string): vscode.Uri {
  ensureProvidersRegistered();
  afterRegistry.set(key, content);
  return vscode.Uri.from({ scheme: AFTER_SCHEME, path: key });
}

// re-export for tests
export const _internal = { beforeRegistry, afterRegistry };
export const BEFORE_SCHEME_NAME = BEFORE_SCHEME;
export const AFTER_SCHEME_NAME = AFTER_SCHEME;
