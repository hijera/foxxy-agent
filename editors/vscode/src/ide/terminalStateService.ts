import * as vscode from "vscode";
import { httpPost } from "../util/http";
import { readSettings } from "../settings";
import {
  appendBounded,
  sameTerminalSnapshot,
  stripAnsi,
  terminalStateRequestBody,
  type TerminalEntry,
  type TerminalSnapshot,
} from "./terminalStatePayload";
import {
  CAPTURE_SENTINEL,
  mergeScreenCapture,
  normalizeScreenCapture,
  screenFromClipboard,
  shouldCaptureScreen,
} from "./terminalCapturePolicy";

const DEBOUNCE_MS = 400;
/** Per-terminal output cap (chars). The backend re-caps defensively too. */
const MAX_OUTPUT_BYTES = 16 * 1024;

interface TrackedTerminal {
  id: string;
  output: string;
  lastCommand: string;
  /** Where `output` came from last: a shell-integration stream or a screen capture. */
  source: "execution" | "screen" | null;
  /** Time of the last shell-integration output chunk. */
  lastExecutionOutputAt: number | null;
  /** Time of the last clipboard screen capture. */
  lastCaptureAt: number | null;
}

/** Reports every open terminal (name, shell, recent output, focus) to the
 *  foxxycode backend (POST /foxxycode/ide/terminal-state) so the model can see
 *  the terminals the user is working in — the same "@terminal" context cline
 *  provides. Output is captured via the shell-integration execution API
 *  (`onDidStartTerminalShellExecution`, VS Code >= 1.93), feature-detected so
 *  the extension keeps its `^1.75.0` engine floor; without shell integration
 *  the terminal list is still reported (with empty output).
 *
 *  Opt-in screen capture (`foxxycode.terminalClipboardCapture`): terminals
 *  that were open before activation, or that have no shell integration, only
 *  expose their buffer through the clipboard (select all → copy → read →
 *  restore, the cline approach). It runs for the *active* terminal when it
 *  changes, when reporting starts, and when the FoxxyCode panel is shown,
 *  throttled and skipped while shell integration has fresh output
 *  (`terminalCapturePolicy.ts`).
 *
 *  Mirrors the lifecycle of `ide/editorStateService.ts`: `startIfNeeded(baseUrl)`
 *  wires the subscriptions and `dispose()` tears them down. Gated by the
 *  `foxxycode.trackTerminals` setting (default true). */
export class TerminalStateService {
  private baseUrl: string | null = null;
  private readonly subscriptions: vscode.Disposable[] = [];
  private debounce: ReturnType<typeof setTimeout> | null = null;
  private last: TerminalSnapshot | null = null;
  private nextId = 1;
  private readonly tracked = new WeakMap<vscode.Terminal, TrackedTerminal>();
  /** Re-entrancy guard for the clipboard round-trip. */
  private capturing = false;

  constructor(private readonly log?: (line: string) => void) {}

  /** Starts (or, after a server restart on a new port, re-points) reporting. */
  startIfNeeded(baseUrl: string): void {
    const rebased = this.baseUrl !== baseUrl;
    this.baseUrl = baseUrl;
    if (this.subscriptions.length === 0) {
      const onChange = (): void => this.schedule();
      this.subscriptions.push(
        vscode.window.onDidOpenTerminal(onChange),
        vscode.window.onDidCloseTerminal((term) => {
          this.tracked.delete(term);
          this.schedule();
        }),
        vscode.window.onDidChangeActiveTerminal(() => {
          this.schedule();
          void this.captureActiveScreen("active-changed");
        }),
      );
      // Shell-integration execution API (VS Code >= 1.93): feature-detected so
      // the 1.75 engine floor still compiles/loads on older hosts.
      const w = vscode.window as unknown as {
        onDidStartTerminalShellExecution?: (
          listener: (e: unknown) => void,
        ) => vscode.Disposable;
      };
      if (typeof w.onDidStartTerminalShellExecution === "function") {
        this.subscriptions.push(
          w.onDidStartTerminalShellExecution((e) => void this.captureExecution(e)),
        );
      }
    }
    if (rebased) this.last = null; // force a resend to the new server
    this.schedule();
    void this.captureActiveScreen("start");
  }

  /** Asks for a screen capture of the active terminal (e.g. when the FoxxyCode
   *  panel becomes visible); subject to the capture policy. */
  requestScreenCapture(reason: string): void {
    void this.captureActiveScreen(reason);
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

  /** Returns (creating if needed) the tracking record for a terminal. */
  private track(terminal: vscode.Terminal): TrackedTerminal {
    let t = this.tracked.get(terminal);
    if (!t) {
      t = {
        id: String(this.nextId++),
        output: "",
        lastCommand: "",
        source: null,
        lastExecutionOutputAt: null,
        lastCaptureAt: null,
      };
      this.tracked.set(terminal, t);
    }
    return t;
  }

  /** Reads the active terminal's visible buffer through the clipboard and
   *  folds it into that terminal's reported output. The clipboard is always
   *  restored, even when a step throws. Only the active terminal can be read:
   *  the workbench commands act on it. A terminal that was never rendered (or
   *  an empty one) copies nothing, which the sentinel makes detectable. */
  private async captureActiveScreen(reason: string): Promise<void> {
    if (!this.baseUrl) return;
    const term = vscode.window.activeTerminal;
    const settings = readSettings();
    const tracked = term ? this.track(term) : null;
    const go = shouldCaptureScreen({
      enabled: settings.trackTerminals && settings.terminalClipboardCapture,
      hasActiveTerminal: !!term,
      capturing: this.capturing,
      now: Date.now(),
      lastCaptureAt: tracked?.lastCaptureAt ?? null,
      lastExecutionOutputAt: tracked?.lastExecutionOutputAt ?? null,
    });
    if (!go || !tracked) return;

    this.capturing = true;
    let saved: string | null = null;
    try {
      saved = await vscode.env.clipboard.readText();
      await vscode.env.clipboard.writeText(CAPTURE_SENTINEL);
      await vscode.commands.executeCommand("workbench.action.terminal.selectAll");
      await vscode.commands.executeCommand("workbench.action.terminal.copySelection");
      const text = screenFromClipboard(await vscode.env.clipboard.readText());
      await vscode.commands.executeCommand("workbench.action.terminal.clearSelection");
      tracked.lastCaptureAt = Date.now();
      if (text !== null) {
        const { output, changed } = mergeScreenCapture(
          tracked.output,
          normalizeScreenCapture(text, MAX_OUTPUT_BYTES),
        );
        if (changed) {
          tracked.output = output;
          tracked.source = "screen";
          this.schedule();
        }
      }
    } catch (e) {
      this.log?.(
        `[foxxycode] terminal capture failed (${reason}): ${(e as Error).message ?? String(e)}`,
      );
    } finally {
      if (saved !== null) {
        try {
          await vscode.env.clipboard.writeText(saved);
        } catch {
          // Nothing more to do: the clipboard API itself is unavailable.
        }
      }
      this.capturing = false;
    }
  }

  /** Consumes a shell execution's output stream into the terminal's buffer. */
  private async captureExecution(e: unknown): Promise<void> {
    const ev = e as {
      terminal?: vscode.Terminal;
      execution?: {
        read?: () => AsyncIterable<string>;
        commandLine?: { value?: string } | string;
      };
    };
    const terminal = ev.terminal;
    const execution = ev.execution;
    if (!terminal || !execution || typeof execution.read !== "function") {
      return;
    }
    const t = this.track(terminal);
    const cmd =
      typeof execution.commandLine === "string"
        ? execution.commandLine
        : execution.commandLine?.value;
    if (cmd) t.lastCommand = cmd;
    try {
      for await (const chunk of execution.read()) {
        t.output = appendBounded(t.output, stripAnsi(String(chunk)), MAX_OUTPUT_BYTES);
        t.source = "execution";
        t.lastExecutionOutputAt = Date.now();
        this.schedule();
      }
    } catch {
      // Stream ended or errored — best-effort.
    }
    this.schedule();
  }

  private schedule(): void {
    if (this.debounce) clearTimeout(this.debounce);
    this.debounce = setTimeout(() => {
      this.debounce = null;
      void this.report();
    }, DEBOUNCE_MS);
  }

  /** Best-effort shell path from a terminal's creation options. */
  private shellOf(terminal: vscode.Terminal): string | undefined {
    const opts = terminal.creationOptions as { shellPath?: string } | undefined;
    return opts?.shellPath || undefined;
  }

  /** Best-effort cwd from shell integration (VS Code >= 1.93), when present. */
  private cwdOf(terminal: vscode.Terminal): string | undefined {
    const si = (terminal as unknown as { shellIntegration?: { cwd?: vscode.Uri } })
      .shellIntegration;
    return si?.cwd?.fsPath || undefined;
  }

  private buildSnapshot(): TerminalSnapshot {
    const active = vscode.window.activeTerminal;
    const terminals: TerminalEntry[] = [];
    for (const term of vscode.window.terminals) {
      const t = this.track(term);
      const entry: TerminalEntry = {
        id: t.id,
        name: term.name,
        output: t.output,
        active: term === active,
      };
      const shell = this.shellOf(term);
      if (shell) entry.shell = shell;
      const cwd = this.cwdOf(term);
      if (cwd) entry.cwd = cwd;
      if (t.lastCommand) entry.lastCommand = t.lastCommand;
      terminals.push(entry);
    }
    return { terminals };
  }

  private async report(): Promise<void> {
    if (!this.baseUrl) return;
    if (!readSettings().trackTerminals) return;

    const snap = this.buildSnapshot();
    if (this.last && sameTerminalSnapshot(this.last, snap)) return;
    this.last = snap;

    const url = `${this.baseUrl.replace(/\/$/, "")}/foxxycode/ide/terminal-state`;
    try {
      await httpPost(url, { body: terminalStateRequestBody(snap) });
    } catch (e) {
      this.log?.(`[foxxycode] terminal-state post failed: ${(e as Error).message ?? String(e)}`);
    }
  }
}
