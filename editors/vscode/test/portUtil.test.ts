import { describe, it, expect } from "vitest";
import * as net from "net";
import {
  awaitPortAvailable,
  isPortAvailable,
  pickFreePort,
  probeHost,
} from "../src/process/portUtil";

/** Binds an ephemeral loopback port and keeps it until `close()` is called. */
function holdPort(): Promise<{ port: number; close: () => Promise<void> }> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.on("error", reject);
    srv.listen(0, "127.0.0.1", () => {
      const addr = srv.address();
      if (!addr || typeof addr !== "object") {
        reject(new Error("no address"));
        return;
      }
      resolve({
        port: addr.port,
        close: () => new Promise<void>((done) => srv.close(() => done())),
      });
    });
  });
}

describe("pickFreePort", () => {
  it("returns a fixed port when valid", async () => {
    expect(await pickFreePort(4242)).toBe(4242);
    expect(await pickFreePort(1)).toBe(1);
    expect(await pickFreePort(65535)).toBe(65535);
  });

  it("rejects out-of-range fixed ports and picks a free one", async () => {
    const p = await pickFreePort(0);
    expect(p).toBeGreaterThan(0);
    expect(p).toBeLessThanOrEqual(65535);
    const p2 = await pickFreePort(70000);
    expect(p2).toBeGreaterThan(0);
    expect(p2).toBeLessThanOrEqual(65535);
    const p3 = await pickFreePort(-1);
    expect(p3).toBeGreaterThan(0);
  });

  it("returns distinct free ports across calls", async () => {
    const a = await pickFreePort(0);
    const b = await pickFreePort(0);
    // Not strictly guaranteed, but in practice always distinct.
    expect(a).not.toBe(b);
  });
});

describe("probeHost", () => {
  it("probes wildcard and empty hosts on loopback", () => {
    expect(probeHost("")).toBe("127.0.0.1");
    expect(probeHost("  ")).toBe("127.0.0.1");
    expect(probeHost("0.0.0.0")).toBe("127.0.0.1");
    expect(probeHost("::")).toBe("127.0.0.1");
  });

  it("keeps an explicit host", () => {
    expect(probeHost(" localhost ")).toBe("localhost");
    expect(probeHost("127.0.0.1")).toBe("127.0.0.1");
  });
});

describe("isPortAvailable", () => {
  it("is true for a free port", async () => {
    const port = await pickFreePort(0);
    expect(await isPortAvailable(port, "127.0.0.1")).toBe(true);
  });

  it("is false while another listener holds the port", async () => {
    const held = await holdPort();
    try {
      expect(await isPortAvailable(held.port, "127.0.0.1")).toBe(false);
      // Wildcard hosts are probed on loopback, where the holder lives.
      expect(await isPortAvailable(held.port, "0.0.0.0")).toBe(false);
      expect(await isPortAvailable(held.port, "")).toBe(false);
    } finally {
      await held.close();
    }
    expect(await isPortAvailable(held.port, "127.0.0.1")).toBe(true);
  });
});

describe("awaitPortAvailable", () => {
  it("returns immediately without sleeping when the port is free", async () => {
    const port = await pickFreePort(0);
    const sleeps: number[] = [];
    const ok = await awaitPortAvailable(port, "127.0.0.1", 50, 10, async (ms) => {
      sleeps.push(ms);
    });
    expect(ok).toBe(true);
    expect(sleeps).toEqual([]);
  });

  it("gives up after the wait budget when the port stays busy", async () => {
    const held = await holdPort();
    const sleeps: number[] = [];
    try {
      const ok = await awaitPortAvailable(held.port, "127.0.0.1", 20, 5, async (ms) => {
        sleeps.push(ms);
        await new Promise((r) => setTimeout(r, ms));
      });
      expect(ok).toBe(false);
      expect(sleeps.length).toBeGreaterThanOrEqual(1);
      for (const ms of sleeps) expect(ms).toBeLessThanOrEqual(5);
    } finally {
      await held.close();
    }
  });

  it("succeeds once the holder releases the port during the wait", async () => {
    const held = await holdPort();
    let sleeps = 0;
    const ok = await awaitPortAvailable(held.port, "127.0.0.1", 5000, 500, async () => {
      sleeps++;
      await held.close();
    });
    expect(ok).toBe(true);
    expect(sleeps).toBe(1);
  });
});
