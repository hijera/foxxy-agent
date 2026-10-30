import { afterEach, describe, expect, it, vi } from "vitest";
import {
  readSwarmLayoutMode,
  SWARM_LAYOUT_STORAGE_KEY,
  writeSwarmLayoutMode,
} from "./layoutMode";

class MapStorage {
  private values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
}

afterEach(() => vi.unstubAllGlobals());

describe("swarm layout preference", () => {
  it("defaults a missing preference to tree", () => {
    expect(readSwarmLayoutMode(new MapStorage())).toBe("tree");
  });

  it.each(["unknown", "GRAPH", " graph ", ""])(
    "defaults the unknown value %j to tree",
    (value) => {
      const storage = new MapStorage();
      storage.setItem(SWARM_LAYOUT_STORAGE_KEY, value);
      expect(readSwarmLayoutMode(storage)).toBe("tree");
    },
  );

  it("reads the legacy star preference as graph", () => {
    const storage = new MapStorage();
    storage.setItem(SWARM_LAYOUT_STORAGE_KEY, "star");
    expect(readSwarmLayoutMode(storage)).toBe("graph");
  });

  it.each(["graph", "tree"] as const)("stores and rereads %s", (mode) => {
    const storage = new MapStorage();
    writeSwarmLayoutMode(mode, storage);
    expect(SWARM_LAYOUT_STORAGE_KEY).toBe("foxxycode_swarm_layout");
    expect(storage.getItem(SWARM_LAYOUT_STORAGE_KEY)).toBe(mode);
    expect(readSwarmLayoutMode(storage)).toBe(mode);
  });

  it("never persists the retired star name", () => {
    const storage = new MapStorage();
    // @ts-expect-error the writer no longer accepts the retired star mode
    writeSwarmLayoutMode("star", storage);
    expect(storage.getItem(SWARM_LAYOUT_STORAGE_KEY)).toBe("graph");
    expect(readSwarmLayoutMode(storage)).toBe("graph");
  });

  it("uses browser localStorage when no storage is supplied", () => {
    const storage = new MapStorage();
    vi.stubGlobal("localStorage", storage);
    writeSwarmLayoutMode("graph");
    expect(storage.getItem(SWARM_LAYOUT_STORAGE_KEY)).toBe("graph");
    expect(readSwarmLayoutMode()).toBe("graph");
  });

  it("reads and writes safely without browser storage", () => {
    vi.stubGlobal("localStorage", undefined);
    expect(readSwarmLayoutMode()).toBe("tree");
    expect(() => writeSwarmLayoutMode("graph")).not.toThrow();
  });

  it("defaults to tree when reading storage throws SecurityError", () => {
    const storage = new MapStorage();
    storage.getItem = () => {
      throw new DOMException("denied", "SecurityError");
    };
    expect(readSwarmLayoutMode(storage)).toBe("tree");
  });

  it.each(["read", "write"])(
    "tolerates a throwing global localStorage getter on default %s",
    (operation) => {
      const descriptor = Object.getOwnPropertyDescriptor(
        globalThis,
        "localStorage",
      );
      try {
        Object.defineProperty(globalThis, "localStorage", {
          configurable: true,
          get() {
            throw new DOMException("Storage access denied", "SecurityError");
          },
        });
        if (operation === "read") {
          expect(readSwarmLayoutMode()).toBe("tree");
        } else {
          expect(() => writeSwarmLayoutMode("graph")).not.toThrow();
        }
      } finally {
        if (descriptor) {
          Object.defineProperty(globalThis, "localStorage", descriptor);
        } else {
          Reflect.deleteProperty(globalThis, "localStorage");
        }
      }
    },
  );
});
