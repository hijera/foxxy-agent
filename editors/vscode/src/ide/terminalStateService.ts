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

const DEBOUNCE_MS = 400;
/** Per-terminal output cap (chars). The backend re-caps defensively too. */
const MAX_OUTPUT_BYTES = 16 * 1024;

interface TrackedTerminal {
  id: string;
  output: string;
  lastCommand: string;
}

/** Reports every open terminal (name, shell, recent output, focus) to the
 *  foxxycode backend (POST /foxxycode/ide/terminal-state) so the model can see
 *  the terminals the user is working in — the same "@terminal" context cline
 *  provides. Output is captured via the shell-integration execution API
 *  (`onDidStartTerminalShellExecution`, VS Code >= 1.93), feature-detected so
 *  the extension keeps its `^1.75.0` engine floor; without shell integration
 *  the terminal list is still reported (with empty output).
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
        vscode.window.onDidChangeActiveTerminal(onChange),
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
      t = { id: String(this.nextId++), output: "", lastCommand: "" };
      this.tracked.set(terminal, t);
    }
    return t;
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
