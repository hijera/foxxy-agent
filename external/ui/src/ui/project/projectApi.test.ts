import { afterEach, describe, expect, it, vi } from "vitest";
import {
  fetchProject,
  pickFolder,
  projectBasename,
  putProject,
} from "./projectApi";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("projectBasename", () => {
  it("handles windows and posix separators", () => {
    expect(projectBasename("H:\\PycharmProjects\\foxxy-agent")).toBe(
      "foxxy-agent",
    );
    expect(projectBasename("/home/user/proj")).toBe("proj");
    expect(projectBasename("/home/user/proj/")).toBe("proj");
    expect(projectBasename("plain")).toBe("plain");
    expect(projectBasename("")).toBe("");
  });
});

describe("fetchProject", () => {
  it("returns the project info", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            path: "H:\\proj",
            source: "project",
            native_picker: true,
          }),
          { status: 200 },
        ),
      ),
    );
    const info = await fetchProject();
    expect(info?.path).toBe("H:\\proj");
    expect(info?.native_picker).toBe(true);
  });

  it("returns null on failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("nope", { status: 500 })),
    );
    expect(await fetchProject()).toBeNull();
  });
});

describe("putProject", () => {
  it("surfaces the backend error message on 400", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(
          JSON.stringify({ error: { message: "not a directory" } }),
          { status: 400 },
        ),
      ),
    );
    const res = await putProject("H:\\bad");
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.error).toBe("not a directory");
    }
  });

  it("returns the updated info on success", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          path: "H:\\proj",
          source: "project",
          native_picker: false,
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    const res = await putProject("H:\\proj");
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.info.source).toBe("project");
    }
    expect(fetchMock).toHaveBeenCalledWith(
      "/foxxycode/project",
      expect.objectContaining({ method: "PUT" }),
    );
  });
});

describe("pickFolder", () => {
  it("maps 501 to unavailable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("no picker", { status: 501 })),
    );
    const res = await pickFolder();
    expect(res && "unavailable" in res).toBe(true);
  });

  it("returns the picked path", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(JSON.stringify({ path: "H:\\picked", cancelled: false }), {
          status: 200,
        }),
      ),
    );
    const res = await pickFolder();
    expect(res && "path" in res && res.path).toBe("H:\\picked");
  });
});
