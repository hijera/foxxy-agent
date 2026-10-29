import { beforeEach, describe, expect, it, vi } from "vitest";

// The token bookkeeping under test reads and writes the environment store;
// the store is mocked here instead of driven through localStorage, so the
// cases can pin the exact value handed to setEnv.
let env: { mode: string; baseUrl: string; token: string; name?: string };
let browserToken = "";
let configDoc: unknown = {};
const setEnvMock = vi.fn();

vi.mock("./remoteEnv", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./remoteEnv")>();
  return {
    ...actual,
    getEnv: () => env,
    setEnv: (next: typeof env) => {
      setEnvMock(next);
      env = next;
    },
    getRemoteToken: () => browserToken,
    localFetch: async () =>
      new Response(JSON.stringify(configDoc), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
  };
});

import {
  refreshConfiguredRemotes,
  resetConfiguredRemotesForTests,
} from "./configuredRemotes";

beforeEach(() => {
  resetConfiguredRemotesForTests();
  setEnvMock.mockClear();
  browserToken = "";
  configDoc = {};
});

describe("syncActiveToken", () => {
  // Rotation is not the only change a config_reloaded can carry: a token
  // deleted from httpserver.remotes must leave the active environment too,
  // or the page keeps probing with the removed token until the remote is
  // chosen again.
  it("drops the active token when the configuration stopped carrying one", async () => {
    env = { mode: "remote", baseUrl: "http://r:1", token: "stale" };
    browserToken = "browser";
    configDoc = {
      httpserver: {
        remotes: [{ name: "office", url: "http://r:1", token: "stale" }],
      },
    };
    await refreshConfiguredRemotes();
    // The token is removed from the entry while the page keeps running.
    configDoc = {
      httpserver: { remotes: [{ name: "office", url: "http://r:1" }] },
    };
    await refreshConfiguredRemotes();
    expect(setEnvMock).toHaveBeenCalledWith({
      mode: "remote",
      baseUrl: "http://r:1",
      token: "browser",
    });
  });

  // An entry that never carried a token is not a removal: the token in use
  // may be one typed into this browser, and absence must not wipe it.
  it("keeps a token the entry never carried", async () => {
    env = { mode: "remote", baseUrl: "http://r:1", token: "typed" };
    configDoc = {
      httpserver: { remotes: [{ name: "office", url: "http://r:1" }] },
    };
    await refreshConfiguredRemotes();
    await refreshConfiguredRemotes();
    expect(setEnvMock).not.toHaveBeenCalled();
  });

  it("takes a rotated token without choosing the remote again", async () => {
    env = { mode: "remote", baseUrl: "http://r:1", token: "old" };
    configDoc = {
      httpserver: {
        remotes: [{ name: "office", url: "http://r:1", token: "new" }],
      },
    };
    await refreshConfiguredRemotes();
    expect(setEnvMock).toHaveBeenCalledWith({
      mode: "remote",
      baseUrl: "http://r:1",
      token: "new",
    });
  });

  it("leaves the environment alone when the token is unchanged", async () => {
    env = { mode: "remote", baseUrl: "http://r:1", token: "same" };
    configDoc = {
      httpserver: {
        remotes: [{ name: "office", url: "http://r:1", token: "same" }],
      },
    };
    await refreshConfiguredRemotes();
    expect(setEnvMock).not.toHaveBeenCalled();
  });

  it("does not touch a remote the configuration does not list", async () => {
    env = { mode: "remote", baseUrl: "http://manual:1", token: "kept" };
    configDoc = { httpserver: { remotes: [] } };
    await refreshConfiguredRemotes();
    expect(setEnvMock).not.toHaveBeenCalled();
  });
});
