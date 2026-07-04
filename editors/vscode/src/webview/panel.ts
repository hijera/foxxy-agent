import * as vscode from "vscode";
import { t } from "../i18n/bundle";
import { currentFoxxyCodeTheme } from "./themeBridge";
import { spaLanguageCode } from "../i18n/bundle";
import { readSettings } from "../settings";

/** Webview host for the foxxycode SPA. Mirrors `editors/intellij/.../ui/FoxxyCodeBrowserPanel.kt`.
 *
 *  VS Code webviews load external URLs only via an `<iframe>` inside `webview.html`,
 *  and the extension host cannot `executeJavaScript` into a cross-origin iframe
 *  (unlike JCEF). Live theme/language switching is therefore done by reloading
 *  the iframe with updated `?theme=` / `?lang=` query parameters — visually
 *  identical to the IntelliJ flow, technically different. Initial load is
 *  flash-free thanks to `?theme=` being applied before first paint. */

const EMBED_ID = "intellij"; // SPA CSS only specialises this id today (see docs/intellij-embedding.md).

export interface PanelControllerOptions {
  extensionUri: vscode.Uri;
  /** Called whenever the iframe URL changes; used to surface the current URL
   *  to the Open-in-Browser command. */
  onUrl?: (url: string) => void;
}

/** Controller over either a `WebviewPanel` (editor area) or a `WebviewView`
 *  (sidebar). Both share the same HTML builder. */
export class FoxxyCodePanelController {
  private base: string | null = null;
  private currentUrl: string | null = null;
  private readonly disposables: vscode.Disposable[] = [];

  constructor(
    private readonly webview: vscode.Webview,
    private readonly target: vscode.WebviewPanel | vscode.WebviewView,
    private readonly opts: PanelControllerOptions,
  ) {
    webview.options = { enableScripts: true, enableForms: true };

    // Live theme + language switching: reload the iframe with updated query params.
    this.disposables.push(
      vscode.window.onDidChangeActiveColorTheme(() => this.refresh()),
      vscode.workspace.onDidChangeConfiguration((e) => {
        if (e.affectsConfiguration("foxxycode.language") || e.affectsConfiguration("foxxycode.followVscodeTheme")) {
          this.refresh();
        }
      }),
    );
  }

  /** Point the iframe at a fresh base URL (e.g. after a process restart on a new port). */
  setBaseUrl(baseUrl: string): void {
    this.base = baseUrl;
    this.render();
  }

  /** Re-render the iframe with current base + current theme/lang (live switching). */
  refresh(): void {
    if (this.base) this.render();
  }

  reload(): void {
    // VS Code webviews: postMessage to ask the iframe to reload itself, or just re-render.
    this.webview.postMessage({ type: "foxxycode:reload" });
    if (this.base) this.render();
  }

  openDevtools(): void {
    void vscode.commands.executeCommand("workbench.action.webview.openDeveloperTools");
  }

  async openInBrowser(): Promise<void> {
    if (this.currentUrl) {
      await vscode.env.openExternal(vscode.Uri.parse(this.currentUrl));
    }
  }

  dispose(): void {
    for (const d of this.disposables) d.dispose();
    this.disposables.length = 0;
  }

  // ---- rendering ------------------------------------------------------------

  private render(): void {
    if (!this.base) return;
    const settings = readSettings();
    const theme = settings.followVscodeTheme ? currentFoxxyCodeTheme() : "dark";
    const lang = spaLanguageCode(settings.language, vscode.env.language);
    const url = appendQueryParams(this.base, { theme, lang, embed: EMBED_ID });
    this.currentUrl = url;
    this.opts.onUrl?.(url);
    this.webview.html = this.buildHtml(url);
  }

  private buildHtml(iframeSrc: string): string {
    // CSP: allow the iframe to load the local foxxycode http server (any loopback port,
    // since the port is auto-picked at runtime). Inline styles are needed for the
    // full-bleed iframe layout.
    const csp = [
      "default-src 'none'",
      "frame-src http://127.0.0.1:* http://localhost:*",
      "style-src 'unsafe-inline'",
      "script-src 'none'",
    ].join("; ");
    return /* html */ `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8" />
<meta http-equiv="Content-Security-Policy" content="${csp}" />
<style>
  html, body { margin: 0; padding: 0; height: 100%; background: var(--vscode-editor-background, #1e1e1e); }
  iframe { display: block; width: 100%; height: 100%; border: 0; }
</style>
</head>
<body>
<iframe id="foxxy" src="${escapeAttr(iframeSrc)}" title="FoxxyCode" allow="clipboard-read; clipboard-write"></iframe>
<script>
  (function () {
    // Polyfill crypto.randomUUID for older embedded Chromium (< 92) — the SPA
    // calls it when creating a chat draft and crashes to a blank page without it.
    try {
      var c = window.crypto || window.msCrypto;
      if (c && typeof c.randomUUID !== "function" && c.getRandomValues) {
        c.randomUUID = function () {
          var b = c.getRandomValues(new Uint8Array(16));
          b[6] = (b[6] & 0x0f) | 0x40;
          b[8] = (b[8] & 0x3f) | 0x80;
          var h = [];
          for (var i = 0; i < 16; i++) h.push((b[i] + 0x100).toString(16).slice(1));
          return h[0]+h[1]+h[2]+h[3]+"-"+h[4]+h[5]+"-"+h[6]+h[7]+"-"+h[8]+h[9]+"-"+h[10]+h[11]+h[12]+h[13]+h[14]+h[15];
        };
      }
    } catch (e) {}
    // Show uncaught errors as an overlay so the page never silently goes blank.
    try {
      if (!window.__foxxycodeErrOverlayInstalled) {
        window.__foxxycodeErrOverlayInstalled = true;
        var show = function (title, detail) {
          try {
            var el = document.getElementById("foxxycode-err-overlay");
            if (!el) {
              el = document.createElement("div");
              el.id = "foxxycode-err-overlay";
              el.style.cssText = "position:fixed;left:0;right:0;bottom:0;z-index:2147483647;max-height:45vh;overflow:auto;background:#7f1d1d;color:#fff;font:12px/1.45 monospace;padding:10px 12px;white-space:pre-wrap;border-top:2px solid #ef4444";
              (document.body || document.documentElement).appendChild(el);
            }
            el.textContent = "FoxxyCode UI error — " + title + "\\n" + (detail || "");
          } catch (e) {}
        };
        window.addEventListener("error", function (ev) {
          show(ev.message || "error", (ev.error && ev.error.stack) ? ev.error.stack : (ev.filename + ":" + ev.lineno));
        });
        window.addEventListener("unhandledrejection", function (ev) {
          var r = ev.reason;
          show("unhandled promise rejection", (r && (r.stack || r.message)) ? (r.stack || r.message) : String(r));
        });
      }
    } catch (e) {}
    // Reload command from the extension host.
    window.addEventListener("message", function (ev) {
      if (ev.data && ev.data.type === "foxxycode:reload") {
        var f = document.getElementById("foxxy");
        if (f) f.src = f.src;
      }
    });
  })();
</script>
</body>
</html>`;
  }
}

/** Append `?key=value&...` to `base`, avoiding duplicate params. */
function appendQueryParams(base: string, params: Record<string, string>): string {
  const url = new URL(base);
  for (const [k, v] of Object.entries(params)) {
    if (!url.searchParams.has(k)) url.searchParams.set(k, v);
  }
  return url.toString();
}

function escapeAttr(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;");
}

// ---- view provider for the activitybar webview view -------------------------

export class FoxxyCodeViewProvider implements vscode.WebviewViewProvider {
  public controller: FoxxyCodePanelController | null = null;

  constructor(
    private readonly extensionUri: vscode.Uri,
    private readonly onUrl: (url: string) => void,
  ) {}

  resolveWebviewView(view: vscode.WebviewView): void {
    this.controller = new FoxxyCodePanelController(view.webview, view, {
      extensionUri: this.extensionUri,
      onUrl: this.onUrl,
    });
    // The base URL will be pushed in by extension.ts once the process is ready.
  }
}
