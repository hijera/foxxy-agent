import { ChildProcess, spawn } from "child_process";
import { httpGet } from "../util/http";
import { awaitPortAvailable, pickFreePort } from "./portUtil";
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

/** A newer start or an explicit stop superseded this launch. */
export class StartCancelledError extends Error {
  constructor() {
    super("FoxxyCode startup cancelled");
    this.name = "StartCancelledError";
  }
}

/** Owns the per-workspace `foxxycode http` subprocess: launch, readiness polling,
 *  restart, shutdown. Mirrors `editors/intellij/.../process/FoxxyCodeProcessManager.kt`. */
export class ProcessManager {
  private child: ChildProcess | null = null;
  private _baseUrl: string | null = null;
  private starting: Promise<StartResult> | null = null;
  private launchGeneration = 0;
  private cancelStart: (() => void) | null = null;

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
    this.stopInternal();
    const generation = this.launchGeneration;
    const cancelled = new Promise<never>((_, reject) => {
      this.cancelStart = () => reject(new StartCancelledError());
    });
    const pending = Promise.race([this.startAndWait(generation), cancelled]).finally(() => {
      if (this.starting === pending) {
        this.starting = null;
        this.cancelStart = null;
      }
    });
    this.starting = pending;
    return pending;
  }

  async restart(): Promise<StartResult> {
    this.stop();
    return this.start();
  }

  private async startAndWait(generation: number): Promise<StartResult> {
    const { settings, extensionPath, workspaceRoot, log } = this.opts;
    const binary = resolveExisting(extensionPath, settings.binaryPath);
    if (!binary) throw new Error(t("process.error.binaryNotFound"));

    const port = await pickFreePort(settings.port);
    this.assertCurrentLaunch(generation);
    const host = settings.host && settings.host.trim() !== "" ? settings.host.trim() : "127.0.0.1";
    // A fixed port can simply be taken - by another VS Code window (the setting
    // is user-wide while the process is per workspace) or by the previous
    // backend that an extension update is still shutting down. Waiting covers
    // the second case; the first one has to be told to the user, because the
    // process would otherwise die on bind and surface as the generic "exited
    // before becoming ready". Mirrors FoxxyCodeProcessManager.kt.
    if (settings.port >= 1 && settings.port <= 65535 && !(await awaitPortAvailable(port, host))) {
      this.assertCurrentLaunch(generation);
      log?.(`[foxxycode] fixed port ${host}:${port} is in use`);
      throw new Error(t("process.error.portInUse", String(port)));
    }
    this.assertCurrentLaunch(generation);

    log?.(`[foxxycode] launching ${host}:${port}`);
    this.opts.onLaunching?.(host, port);

    const args = ["http", "-H", host, "-P", String(port)];
    if (workspaceRoot) args.push("--cwd", workspaceRoot);
    if (settings.home && settings.home.trim() !== "") args.push("--home", settings.home.trim());
    // Panels default to guarded planning: the model may not leave plan mode itself.
    args.push(`--plan-no-self-run=${settings.planNoSelfRun ? "true" : "false"}`);
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
        if (this.child === child) {
          this.child = null;
          this._baseUrl = null;
        }
        reject(
          new Error(`${t("process.error.exitedBeforeReady")} — ${err.message}`),
        );
      });
      child.on("exit", (code) => {
        log?.(`[foxxycode] process exited code=${code}`);
        // A stopped backend may exit after its replacement has already spawned.
        // Its callback owns only its own launch, never the replacement's state.
        if (this.child === child) {
          this.child = null;
          this._baseUrl = null;
        }
        reject(new Error(t("process.error.exitedBeforeReady")));
      });

      void this.waitForReady(baseUrl, child, generation)
        .then(() => {
          this.assertLiveChild(child, generation);
          this._baseUrl = baseUrl;
          resolve({ baseUrl });
        })
        .catch((e: Error) => {
          if (this.child === child) {
            this.child = null;
            this._baseUrl = null;
            if (!child.killed) {
              try {
                child.kill();
              } catch {
                // Preserve the startup failure if termination also fails.
              }
            }
          }
          reject(e);
        });
    });
  }

  /** Polls `GET /v1/models` until 2xx-4xx (server accepting requests), 30s deadline. */
  private async waitForReady(baseUrl: string, child: ChildProcess, generation: number): Promise<void> {
    const probe = baseUrl + "v1/models";
    const deadline = Date.now() + 30_000;
    let lastError = "timeout";
    while (Date.now() < deadline) {
      this.assertLiveChild(child, generation);
      try {
        const res = await httpGet(probe, 1500);
        if (res.status >= 200 && res.status <= 499) {
          this.assertLiveChild(child, generation);
          return;
        }
      } catch (e) {
        this.assertCurrentLaunch(generation);
        lastError = (e as Error).message ?? String(e);
      }
      await sleep(300);
    }
    throw new Error(t("process.error.notReady", lastError));
  }

  private assertCurrentLaunch(generation: number): void {
    if (generation !== this.launchGeneration) throw new StartCancelledError();
  }

  private assertLiveChild(child: ChildProcess, generation: number): void {
    this.assertCurrentLaunch(generation);
    if (this.child !== child || child.killed || child.exitCode !== null || child.signalCode !== null) {
      throw new Error(t("process.error.exitedBeforeReady"));
    }
  }

  stop(): void {
    this.stopInternal();
  }

  private stopInternal(): void {
    this.launchGeneration++;
    const cancel = this.cancelStart;
    this.cancelStart = null;
    this.starting = null;
    cancel?.();
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
