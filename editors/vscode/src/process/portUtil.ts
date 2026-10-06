import * as net from "net";

/** Returns `fixed` when it is a valid port, otherwise a free ephemeral port.
 *  Async because Node's `net.createServer().listen(0)` is non-blocking;
 *  mirrors the intent of `editors/intellij/.../process/PortUtil.kt`. */
export async function pickFreePort(fixed: number = 0): Promise<number> {
  if (fixed >= 1 && fixed <= 65535) return fixed;
  return new Promise<number>((resolve, reject) => {
    const srv = net.createServer();
    srv.unref();
    srv.on("error", reject);
    srv.listen(0, "127.0.0.1", () => {
      const addr = srv.address();
      if (addr && typeof addr === "object") {
        const port = addr.port;
        srv.close(() => resolve(port));
      } else {
        srv.close();
        reject(new Error("could not pick a free port"));
      }
    });
  });
}

/** How long a fixed port is given to free up before the start is abandoned:
 *  long enough for the previous backend (an extension update, a restart) to
 *  release it, short enough that a port taken by another window is reported
 *  promptly. Mirrors `PortUtil.FIXED_PORT_WAIT_MS`. */
export const FIXED_PORT_WAIT_MS = 3000;
/** Poll interval for `awaitPortAvailable`. */
export const FIXED_PORT_RETRY_MS = 500;

/** Host to probe: wildcard / empty hosts are probed on loopback, which is what
 *  the backend binds through anyway and never needs an external interface. */
export function probeHost(host: string): string {
  const h = (host ?? "").trim();
  if (h === "" || h === "0.0.0.0" || h === "::") return "127.0.0.1";
  return h;
}

/** True when `port` can be bound on `host` right now.
 *
 *  Node cannot turn SO_REUSEADDR off on Linux/macOS, so a port left in
 *  TIME_WAIT reads as free. That is fine: the Go backend binds with the same
 *  flag and would succeed too. A port with a live listener fails with
 *  EADDRINUSE on every platform, which is the case this exists to detect;
 *  EACCES (privileged port) is reported as unavailable as well. */
export function isPortAvailable(port: number, host: string): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    const srv = net.createServer();
    srv.unref();
    srv.once("error", () => resolve(false));
    srv.listen({ port, host: probeHost(host), exclusive: true }, () => {
      srv.close(() => resolve(true));
    });
  });
}

function defaultSleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Polls `isPortAvailable` until it succeeds or `waitMs` elapses. `sleep` is
 *  injectable for tests. Mirrors `PortUtil.awaitAvailable` in the IntelliJ
 *  plugin. */
export async function awaitPortAvailable(
  port: number,
  host: string,
  waitMs: number = FIXED_PORT_WAIT_MS,
  intervalMs: number = FIXED_PORT_RETRY_MS,
  sleep: (ms: number) => Promise<void> = defaultSleep,
): Promise<boolean> {
  const deadline = Date.now() + waitMs;
  for (;;) {
    if (await isPortAvailable(port, host)) return true;
    const left = deadline - Date.now();
    if (left <= 0) return false;
    await sleep(Math.min(intervalMs, left));
  }
}
