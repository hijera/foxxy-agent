import { EventEmitter } from "node:events";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProcessManager } from "../src/process/processManager";
import type { FoxxyCodeSettings } from "../src/settings";

const stubs = vi.hoisted(() => ({ spawn: vi.fn(), httpGet: vi.fn(), pickPort: vi.fn() }));
vi.mock("child_process", () => ({ spawn: stubs.spawn }));
vi.mock("../src/binary/binaryResolver", () => ({ resolveExisting: () => "/fake/foxxycode" }));
vi.mock("../src/util/http", () => ({ httpGet: stubs.httpGet }));
vi.mock("../src/process/portUtil", () => ({ pickFreePort: stubs.pickPort, awaitPortAvailable: async () => true }));

class Backend extends EventEmitter {
  readonly stdout = new EventEmitter();
  readonly stderr = new EventEmitter();
  killed = false;
  exitCode: number | null = null;
  signalCode: string | null = null;
  readonly kill = vi.fn(() => { this.killed = true; return true; });
  exit(): void { this.exitCode = 0; this.emit("exit", 0); }
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
const ready = { status: 200, body: "{}" };
let manager: ProcessManager;
let backends: Backend[];
beforeEach(() => {
  backends = [];
  vi.resetAllMocks();
  stubs.spawn.mockImplementation(() => { const child = new Backend(); backends.push(child); return child; });
  stubs.pickPort.mockImplementation(async () => 41000 + backends.length);
  stubs.httpGet.mockResolvedValue(ready);
  manager = new ProcessManager({ extensionPath: "/extension", workspaceRoot: "/workspace", settings: { binaryPath: "", host: "127.0.0.1", port: 0, home: "", extraArgs: "", planNoSelfRun: true } as FoxxyCodeSettings });
});
afterEach(() => manager.dispose());

describe("backend launch ownership", () => {
  it("keeps the replacement running when the previous backend exits", async () => {
    await manager.start();
    const first = backends[0];
    const replacement = await manager.restart();
    first.exit();
    expect(manager.isRunning).toBe(true);
    expect(manager.baseUrl).toBe(replacement.baseUrl);
    manager.stop();
    expect(backends[1].kill).toHaveBeenCalledOnce();
  });

  it("does not reject replacement readiness when the previous backend exits", async () => {
    await manager.start();
    const readiness = deferred<typeof ready>();
    stubs.httpGet.mockReturnValueOnce(readiness.promise);
    const replacement = manager.restart();
    await vi.waitFor(() => expect(backends).toHaveLength(2));
    backends[0].exit();
    readiness.resolve(ready);
    await expect(replacement).resolves.toEqual({ baseUrl: "http://127.0.0.1:41001/" });
    expect(manager.isRunning).toBe(true);
  });

  it("cancels an in-flight start and ignores its late readiness after a new start", async () => {
    const oldReady = deferred<typeof ready>();
    stubs.httpGet.mockReturnValueOnce(oldReady.promise);
    const oldStart = manager.start();
    const cancelled = expect(oldStart).rejects.toThrow();
    await vi.waitFor(() => expect(backends).toHaveLength(1));
    manager.stop();
    const newReady = deferred<typeof ready>();
    stubs.httpGet.mockReturnValueOnce(newReady.promise);
    const replacement = manager.start();
    await vi.waitFor(() => expect(backends).toHaveLength(2));
    oldReady.resolve(ready);
    await cancelled;
    expect(manager.baseUrl).toBeNull();
    newReady.resolve(ready);
    await expect(replacement).resolves.toEqual({ baseUrl: "http://127.0.0.1:41001/" });
    expect(manager.isRunning).toBe(true);
  });

  it("never spawns after stop while a port selection is pending", async () => {
    const port = deferred<number>();
    stubs.pickPort.mockReturnValueOnce(port.promise);
    const start = manager.start();
    const cancelled = expect(start).rejects.toThrow();
    manager.stop();
    port.resolve(42000);
    await cancelled;
    expect(stubs.spawn).not.toHaveBeenCalled();
  });

  it("reports a current spawn error and allows another start", async () => {
    const readiness = deferred<typeof ready>();
    stubs.httpGet.mockReturnValueOnce(readiness.promise);
    const start = manager.start();
    const failed = expect(start).rejects.toThrow("EACCES");
    await vi.waitFor(() => expect(backends).toHaveLength(1));
    backends[0].emit("error", new Error("EACCES"));
    await failed;
    expect(manager.isRunning).toBe(false);
    const replacement = await manager.start();
    readiness.resolve(ready);
    expect(manager.baseUrl).toBe(replacement.baseUrl);
  });

  it("clears state when the current ready backend exits", async () => {
    await manager.start();
    backends[0].exit();
    expect(manager.baseUrl).toBeNull();
    expect(manager.isRunning).toBe(false);
  });

  it("shares one launch between concurrent starts", async () => {
    const readiness = deferred<typeof ready>();
    stubs.httpGet.mockReturnValueOnce(readiness.promise);
    const first = manager.start();
    const second = manager.start();
    await vi.waitFor(() => expect(backends).toHaveLength(1));
    readiness.resolve(ready);
    expect(await first).toEqual(await second);
    expect(stubs.spawn).toHaveBeenCalledOnce();
  });
});
