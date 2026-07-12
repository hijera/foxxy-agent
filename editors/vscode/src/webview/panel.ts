import * as vscode from "vscode";
import { t, spaLanguageCode } from "../i18n/bundle";
import { currentFoxxyCodeTheme } from "./themeBridge";
import { readSettings } from "../settings";
import { info } from "../notifications";

/** Webview host for the foxxycode SPA. Mirrors `editors/intellij/.../ui/FoxxyCodeBrowserPanel.kt`,
 *  structurally modelled on the working `coddy-vscode/src/coddyView.ts`.
 *
 *  VS Code webviews load external URLs only via an `<iframe>` inside `webview.html`,
 *  and the extension host cannot `executeJavaScript` into a cross-origin iframe
 *  (unlike JCEF). Live theme/language switching is therefore done by reloading
 *  the iframe with updated `?theme=` / `?lang=` parameters — visually identical
 *  to the IntelliJ flow, technically different. Initial load is flash-free
 *  thanks to `?theme=` being applied before first paint.
 *
 *  The iframe src is run through `vscode.env.asExternalUri` so it works in
 *  Remote SSH / Codespaces / WSL where `127.0.0.1:<port>` is forwarded. */

const EMBED_ID = "intellij"; // SPA CSS only specialises this id today (see docs/intellij-embedding.md).

export interface PanelControllerOptions {
  extensionUri: vscode.Uri;
  /** Called whenever the iframe URL changes; used to surface the current URL
   *  to the Open-in-Browser command. */
  onUrl?: (url: string) => void;
  /** Called when the user clicks Retry in an error view. */
  onRetry?: () => void;
  /** Called when the user clicks Open Settings in an error view. */
  onOpenSettings?: () => void;
  /** Called when the embedded SPA switches its UI language (the user flipped
   *  the single app-wide switcher in SPA Settings → General). The SPA already
   *  re-rendered itself; only extension chrome needs to follow — no reload. */
  onSpaLocale?: (locale: "en" | "ru") => void;
}

/** Controller over either a `WebviewPanel` (editor area) or a `WebviewView`
 *  (sidebar). Both share the same HTML builder. The controller can show three
 *  kinds of content: a status message (while the server boots), an error
 *  message (with Retry / Open Settings buttons), or the iframe itself. */
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

    // Receive Retry / Open Settings clicks from the error HTML, plus locale
    // changes forwarded from the embedded SPA by the wrapper script.
    webview.onDidReceiveMessage((msg: { type?: string; locale?: string }) => {
      switch (msg?.type) {
        case "foxxycode:retry":
          this.opts.onRetry?.();
          break;
        case "foxxycode:openSettings":
          this.opts.onOpenSettings?.();
          break;
        case "foxxycode:reload":
          this.reload();
          break;
        case "foxxycode:locale":
          if (msg.locale === "en" || msg.locale === "ru") {
            this.opts.onSpaLocale?.(msg.locale);
          }
          break;
      }
    }, undefined, this.disposables);

    // Live theme switching: reload the iframe with updated query params.
    // (Language has no VS Code setting — it follows the backend config and the
    // SPA-originated foxxycode:locale message above.)
    this.disposables.push(
      vscode.window.onDidChangeActiveColorTheme(() => this.refresh()),
      vscode.workspace.onDidChangeConfiguration((e) => {
        if (e.affectsConfiguration("foxxycode.followVscodeTheme")) {
          this.refresh();
        }
      }),
    );
  }

  /** Show a status message while the server is booting (no iframe yet). */
  showStatus(message: string): void {
    this.webview.html = this.messageHtml(escapeHtml(message), false, this.activeHtmlLang());
  }

  /** Show an error message with Retry / Open Settings buttons. */
  showError(message: string): void {
    this.webview.html = this.messageHtml(escapeHtml(message), true, this.activeHtmlLang());
  }

  /** Point the iframe at a fresh base URL (e.g. after a process restart on a new port).
   *  The URL is run through `asExternalUri` so it works in remote workspaces. */
  async setBaseUrl(baseUrl: string): Promise<void> {
    this.base = baseUrl;
    await this.render();
  }

  /** Re-render the iframe with current base + current theme/lang (live switching). */
  async refresh(): Promise<void> {
    if (this.base) await this.render();
  }

  reload(): void {
    if (this.base) {
      void this.render();
    }
  }

  openDevtools(): void {
    void vscode.commands.executeCommand("workbench.action.webview.openDeveloperTools");
  }

  async openInBrowser(): Promise<void> {
    if (this.currentUrl) {
      await vscode.env.openExternal(vscode.Uri.parse(this.currentUrl));
      return;
    }
    const fallbackUrl = "https://github.com/hijera/foxxy-agent";
    const openLabel = t("process.button.openUrl", fallbackUrl);
    const choice = await info(t("process.fallback.unavailable"), openLabel);
    if (choice === openLabel) {
      await vscode.env.openExternal(vscode.Uri.parse(fallbackUrl));
    }
  }
  dispose(): void {
    for (const d of this.disposables) d.dispose();
    this.disposables.length = 0;
  }

  // ---- rendering ------------------------------------------------------------

  private async render(): Promise<void> {
    if (!this.base) return;
    const settings = readSettings();
    const theme = settings.followVscodeTheme ? currentFoxxyCodeTheme() : "dark";
    const lang = spaLanguageCode(vscode.env.language);
    const localUrl = appendQueryParams(this.base, { theme, lang, embed: EMBED_ID });
    // asExternalUri converts http://127.0.0.1:PORT/ to a forwarded URI in remote
    // workspaces (Remote SSH / Codespaces / WSL). Locally it returns the same URL.
    let external: vscode.Uri;
    try {
      external = await vscode.env.asExternalUri(vscode.Uri.parse(localUrl));
    } catch {
      external = vscode.Uri.parse(localUrl);
    }
    const src = external.toString(true);
    this.currentUrl = src;
    this.opts.onUrl?.(src);
    this.webview.html = this.frameHtml(src, lang);
  }

  private activeHtmlLang(): "en" | "ru" {
    return spaLanguageCode(vscode.env.language);
  }

  private frameHtml(src: string, lang: "en" | "ru"): string {
    // CSP: allow the iframe to load the loopback foxxycode http server on any
    // auto-picked port, plus https for remote-forwarded URIs. Inline styles
    // are needed for the full-bleed iframe layout.
    const csp = [
      "default-src 'none'",
      "frame-src http://127.0.0.1:* http://localhost:* https:",
      "style-src 'unsafe-inline'",
      "script-src 'nonce-" + this.nonce + "'",
    ].join("; ");
    return /* html */ `<!DOCTYPE html>
<html lang="${lang}">
<head>
<meta charset="UTF-8" />
<meta http-equiv="Content-Security-Policy" content="${csp}" />
<style>
  html, body { margin: 0; padding: 0; height: 100%; overflow: hidden; background: var(--vscode-editor-background, #1e1e1e); }
  iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; }
</style>
</head>
<body>
<iframe id="foxxy" src="${escapeAttr(src)}" title="FoxxyCode" allow="clipboard-read; clipboard-write; fullscreen"></iframe>
<script nonce="${this.nonce}">
  (function () {
    // Forward SPA locale changes (embedLocaleBridge postMessage from the
    // cross-origin iframe) to the extension host so command titles follow the
    // single app-wide language switcher without reloading the iframe.
    try {
      var vscodeApi = acquireVsCodeApi();
      var frame = document.getElementById("foxxy");
      window.addEventListener("message", function (ev) {
        if (
          frame && ev.source === frame.contentWindow && ev.data &&
          ev.data.type === "foxxycode:locale" &&
          (ev.data.locale === "en" || ev.data.locale === "ru")
        ) {
          vscodeApi.postMessage({ type: "foxxycode:locale", locale: ev.data.locale });
        }
      });
    } catch (e) {}
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
  })();
</script>
</body>
</html>`;
  }

  private messageHtml(message: string, isError: boolean, lang: "en" | "ru"): string {
    const actions = isError
      ? `<div class="actions">
           <button id="retry">${escapeHtml(t("process.button.retry"))}</button>
           <button id="settings">${escapeHtml(t("process.button.openSettings"))}</button>
         </div>`
      : "";
    const title = isError ? escapeHtml(t("process.error.startFailed")) : "";
    return /* html */ `<!DOCTYPE html>
<html lang="${lang}">
<head>
<meta charset="UTF-8" />
<meta http-equiv="Content-Security-Policy"
      content="default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-${this.nonce}';" />
<style>
  body { font-family: var(--vscode-font-family); color: var(--vscode-foreground);
         padding: 16px; line-height: 1.5; }
  .title { font-weight: 600; margin-bottom: 8px; }
  .msg { white-space: pre-wrap; opacity: 0.9; }
  .actions { margin-top: 14px; display: flex; gap: 8px; }
  button { background: var(--vscode-button-background); color: var(--vscode-button-foreground);
           border: none; padding: 6px 12px; cursor: pointer; border-radius: 2px; }
  button:hover { background: var(--vscode-button-hoverBackground); }
</style>
</head>
<body>
  ${title ? `<div class="title">${title}</div>` : ''}
  <div class="msg">${message}</div>
  ${actions}
  <script nonce="${this.nonce}">
    const vscode = acquireVsCodeApi();
    const retry = document.getElementById('retry');
    const settings = document.getElementById('settings');
    if (retry) retry.addEventListener('click', () => vscode.postMessage({ type: 'foxxycode:retry' }));
    if (settings) settings.addEventListener('click', () => vscode.postMessage({ type: 'foxxycode:openSettings' }));
  </script>
</body>
</html>`;
  }

  private get nonce(): string {
    // Stable per-instance nonce; regenerated on each render would also be fine
    // but reusing avoids re-allocating on every theme switch.
    if (!this._nonce) {
      this._nonce = makeNonce();
    }
    return this._nonce;
  }

  private _nonce = "";
}

/** Append `?key=value&...` to `base`, avoiding duplicate params. */
function appendQueryParams(base: string, params: Record<string, string>): string {
  const url = new URL(base);
  for (const [k, v] of Object.entries(params)) {
    if (!url.searchParams.has(k)) url.searchParams.set(k, v);
  }
  return url.toString();
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function escapeAttr(s: string): string {
  return escapeHtml(s).replace(/"/g, "&quot;");
}

function makeNonce(): string {
  const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  let out = "";
  for (let i = 0; i < 32; i++) {
    out += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return out;
}
