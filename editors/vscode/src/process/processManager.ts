import { ChildProcess, spawn } from "child_process";
import { httpGet } from "../util/http";
import { pickFreePort } from "./portUtil";
import { resolveExisting } from "../binary/binaryResolver";
import { FoxxyCodeSettings } from "../settings";
import { t } from "../i18n/bundle";
import { ProxyEnv, withProxyEnv } from "./proxyEnv";

export interface ProcessManagerOptions {
  extensionPath: string;
  workspaceRoot: string | undefined;
  /** Snapshot of settings at the moment of `start()`/`restart()`. */
  settings: FoxxyCodeSettings;
  /** Proxy variables derived from the editor's HTTP proxy settings. */
  proxyEnv?: ProxyEnv;
  /** Logger sink; lines from foxxycode stdout/stderr are forwarded here. */
  log?: (line: string) => void;
  /** Called once host/port are known, before readiness polling begins. */
  onLaunching?: (host: string, port: number) => void;
}

export interface StartResult {
  baseUrl: string;
}

/** Owns the per-workspace `foxxycode http` subprocess: launch, readiness polling,
 *  restart, shutdown. Mirrors `editors/intellij/.../process/FoxxyCodeProcessManager.kt`. */
export class ProcessManager {
  private child: ChildProcess | null = null;
  private _baseUrl: string | null = null;
  private starting: Promise<StartResult> | null = null;
  /** Rejects the in-flight `startAndWait` when spawn fails or the process exits early. */
  private startPromiseReject: ((e: Error) => void) | null = null;

  constructor(private readonly opts: ProcessManagerOptions) {}

  updateLaunchOptions(settings: FoxxyCodeSettings, proxyEnv?: ProxyEnv): void {
    this.opts.settings = settings;
    this.opts.proxyEnv = proxyEnv;
  }

  get baseUrl(): string | null {
    return this._baseUrl;
  }

  get isRunning(): boolean {
    return this.child !== null && !this.child.killed;
  }

  /** Resolves once the server is ready; concurrent callers share the same in-flight start. */
  async start(): Promise<StartResult> {
    if (this.isRunning && this._baseUrl) return { baseUrl: this._baseUrl };
    if (this.starting) return this.starting;
    this.starting = this.startAndWait().finally(() => {
      this.starting = null;
    });
    return this.starting;
  }

  async restart(): Promise<StartResult> {
    this.stop();
    return this.start();
  }

  private startAndWait(): Promise<StartResult> {
    this.stopInternal();

    const { settings, extensionPath, workspaceRoot, log } = this.opts;
    const binary = resolveExisting(extensionPath, settings.binaryPath);
    if (!binary) return Promise.reject(new Error(t("process.error.binaryNotFound")));

    return pickFreePort(settings.port).then((port) => {
      const host = settings.host && settings.host.trim() !== "" ? settings.host.trim() : "127.0.0.1";
      log?.(`[foxxycode] launching ${host}:${port}`);
      this.opts.onLaunching?.(host, port);

      const args = ["http", "-H", host, "-P", String(port)];
      if (workspaceRoot) args.push("--cwd", workspaceRoot);
      if (settings.home && settings.home.trim() !== "") args.push("--home", settings.home.trim());
      if (settings.extraArgs && settings.extraArgs.trim() !== "") {
        args.push(...splitArgs(settings.extraArgs));
      }

      log?.(`[foxxycode] launching ${binary} ${args.join(" ")}`);
      const child = spawn(binary, args, {
        cwd: workspaceRoot ?? undefined,
        env: withProxyEnv(process.env, this.opts.proxyEnv ?? {}),
        stdio: ["ignore", "pipe", "pipe"],
        windowsHide: true,
      });
      this.child = child;

      const baseUrl = `http://${host}:${port}/`;

      return new Promise<StartResult>((resolve, reject) => {
        this.startPromiseReject = reject;

        child.stdout.on("data", (chunk: Buffer) => {
          for (const line of chunk.toString("utf8").split(/\r?\n/)) {
            if (line.length) log?.(`[foxxycode] ${line}`);
          }
        });
        child.stderr.on("data", (chunk: Buffer) => {
          for (const line of chunk.toString("utf8").split(/\r?\n/)) {
            if (line.length) log?.(`[foxxycode] ${line}`);
          }
        });
        child.on("error", (err: Error) => {
          log?.(`[foxxycode] spawn error: ${err.message}`);
          this.child = null;
          this._baseUrl = null;
          this.rejectStartPromise(
            new Error(`${t("process.error.exitedBeforeReady")} — ${err.message}`),
          );
        });
        child.on("exit", (code) => {
          log?.(`[foxxycode] process exited code=${code}`);
          this.child = null;
          this._baseUrl = null;
          if (this.startPromiseReject) {
            this.rejectStartPromise(new Error(t("process.error.exitedBeforeReady")));
          }
        });

        void this.waitForReady(baseUrl)
          .then(() => {
            this.startPromiseReject = null;
            this._baseUrl = baseUrl;
            resolve({ baseUrl });
          })
          .catch((e: Error) => {
            this.rejectStartPromise(e);
          });
      });
    });
  }

  /** Polls `GET /v1/models` until 2xx-4xx (server accepting requests), 30s deadline. */
  private async waitForReady(baseUrl: string): Promise<void> {
    const probe = baseUrl + "v1/models";
    const deadline = Date.now() + 30_000;
    let lastError = "timeout";
    while (Date.now() < deadline) {
      if (!this.isRunning) {
        throw new Error(t("process.error.exitedBeforeReady"));
      }
      try {
        const res = await httpGet(probe, 1500);
        if (res.status >= 200 && res.status <= 499) return;
      } catch (e) {
        lastError = (e as Error).message ?? String(e);
      }
      await sleep(300);
    }
    throw new Error(t("process.error.notReady", lastError));
  }

  private rejectStartPromise(err: Error): void {
    const rej = this.startPromiseReject;
    this.startPromiseReject = null;
    rej?.(err);
  }

  stop(): void {
    this.stopInternal();
  }

  private stopInternal(): void {
    const c = this.child;
    this.child = null;
    this._baseUrl = null;
    if (c && !c.killed) {
      try {
        c.kill();
      } catch {
        // ignore
      }
    }
  }

  dispose(): void {
    this.stopInternal();
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Naive shell-style splitter for the `foxxycode.extraArgs` string. */
function splitArgs(s: string): string[] {
  const out: string[] = [];
  const re = /"([^"]*)"|'([^']*)'|(\S+)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(s)) !== null) {
    out.push(m[1] ?? m[2] ?? m[3]);
  }
  return out;
}
