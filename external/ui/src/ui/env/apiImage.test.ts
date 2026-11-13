import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import { apiImageRequest, useApiImageSrc } from "./apiImage";
import { setEnv } from "./remoteEnv";

afterEach(() => setEnv({ mode: "local" }));

const thumb = "/foxxycode/sessions/s1/assets/pasted-1.png/thumbnail";

test("on the local origin an <img> loads the server's URL itself", () => {
  setEnv({ mode: "local" });
  expect(apiImageRequest(thumb)).toBeNull();
});

test("in a remote environment the URL is asked of it, the token in a header", () => {
  setEnv({ mode: "remote", baseUrl: "http://relay.example/swarm/nodes/node/", token: "tok" });
  const req = apiImageRequest(thumb);
  expect(req?.url).toBe("http://relay.example/swarm/nodes/node" + thumb);
  expect(new Headers(req?.init.headers).get("Authorization")).toBe("Bearer tok");
  expect(req?.url).not.toContain("tok");
});

test("a remote environment without a token sends no Authorization header", () => {
  setEnv({ mode: "remote", baseUrl: "http://node.example", token: "" });
  const req = apiImageRequest(thumb);
  expect(req?.url).toBe("http://node.example" + thumb);
  expect(new Headers(req?.init.headers).has("Authorization")).toBe(false);
});

test("what is not the server's API loads as it is, remote or not", () => {
  setEnv({ mode: "remote", baseUrl: "http://relay.example/swarm/nodes/node", token: "tok" });
  for (const url of [
    "blob:http://relay.example/5f1c",
    "data:image/png;base64,aGVsbG8=",
    "https://raw.githubusercontent.com/org/repo/main/shot.png",
    "/assets/foxxycode-logo.svg",
  ]) {
    expect(apiImageRequest(url)).toBeNull();
  }
});

// The object URL a remote environment's bytes were shown from is released when
// the environment changes - and must not stay behind as the src an <img> is
// given: a revoked blob URL renders as a broken image, where the hook promises
// no src at all until a refetch arrives.
test("an environment change drops the src it just released", async () => {
  setEnv({ mode: "remote", baseUrl: "http://node.example", token: "one" });
  let made = 0;
  const created: string[] = [];
  const revoked: string[] = [];
  Object.defineProperties(URL, {
    createObjectURL: {
      configurable: true,
      value: () => {
        const url = `blob:mock-${made++}`;
        created.push(url);
        return url;
      },
    },
    revokeObjectURL: {
      configurable: true,
      value: (url: string) => {
        revoked.push(url);
      },
    },
  });
  // The second read is held open, so the hook is observed in the window the bug
  // lived in: the environment changed, the bytes have not arrived yet.
  let releaseSecond: (() => void) | null = null;
  const fetchMock = vi.fn(async () => {
    if (fetchMock.mock.calls.length > 1) {
      return new Promise<Response>((resolve) => {
        releaseSecond = () => resolve(new Response(new Blob(["y"]), { status: 200 }));
      });
    }
    return new Response(new Blob(["x"]), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
  try {
    const { result, unmount } = renderHook(() => useApiImageSrc(thumb));
    await waitFor(() => expect(result.current).toBe("blob:mock-0"));
    expect(revoked).toEqual([]);

    await act(async () => {
      setEnv({ mode: "remote", baseUrl: "http://node.example", token: "two" });
    });
    expect(revoked).toContain("blob:mock-0");
    expect(result.current).toBeUndefined();

    await act(async () => {
      releaseSecond?.();
    });
    await waitFor(() => expect(result.current).toBe("blob:mock-1"));
    unmount();
    expect(revoked).toContain("blob:mock-1");
  } finally {
    vi.unstubAllGlobals();
    delete (URL as unknown as Record<string, unknown>).createObjectURL;
    delete (URL as unknown as Record<string, unknown>).revokeObjectURL;
  }
});
