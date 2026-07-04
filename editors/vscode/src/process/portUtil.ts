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
