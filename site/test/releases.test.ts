import { describe, expect, it, vi } from "vitest";
import fixture from "./fixtures/releases.json";
import { fetchReleases, pickDownloads, publishedReleases, type Release } from "../scripts/lib/releases.mjs";

const release = (tag: string, assets: string[], extra: Partial<Release> = {}): Release => ({
  tag_name: tag,
  html_url: `https://github.com/o/r/releases/tag/${tag}`,
  published_at: "2026-09-15T00:00:00Z",
  draft: false,
  prerelease: false,
  assets: assets.map((name) => ({ name, browser_download_url: `https://example.com/${tag}/${name}`, size: 1024 })),
  ...extra,
});

describe("pickDownloads", () => {
  it("links every card of the real release fixture", () => {
    const downloads = pickDownloads(fixture as Release[]);
    expect(downloads.vscode?.name).toBe("foxxycode-vscode-0.2.93.vsix");
    expect(downloads.intellij?.name).toBe("foxxycode-intellij-0.2.93.zip");
    expect(downloads.desktop?.name).toBe("foxxycode-desktop_0.2.93_windows_amd64.zip");
    expect(downloads.cliWindows?.name).toBe("foxxycode_0.2.93_windows_amd64.zip");
  });

  it("falls back to the previous release when the newest lacks an asset", () => {
    const downloads = pickDownloads([
      release("0.2.11", ["foxxycode-intellij-0.2.11.zip"]),
      release("0.2.10", ["foxxycode-vscode-0.2.10.vsix", "foxxycode-intellij-0.2.10.zip"]),
    ]);
    expect(downloads.intellij?.version).toBe("0.2.11");
    expect(downloads.vscode?.version).toBe("0.2.10");
    expect(downloads.desktop).toBeNull();
  });

  it("ignores drafts, prereleases and tags that are not X.Y.Z", () => {
    const list = [
      release("0.3.0", ["foxxycode-vscode-0.3.0.vsix"], { draft: true }),
      release("0.2.99", ["foxxycode-vscode-0.2.99.vsix"], { prerelease: true }),
      release("v1.0.0", ["foxxycode-vscode-1.0.0.vsix"]),
      release("0.2.9", ["foxxycode-vscode-0.2.9.vsix"]),
      release("0.2.10", ["foxxycode-vscode-0.2.10.vsix"]),
    ];
    expect(publishedReleases(list).map((r) => r.tag_name)).toEqual(["0.2.10", "0.2.9"]);
    expect(pickDownloads(list).vscode?.version).toBe("0.2.10");
  });

  it("does not take the Linux CLI archive for the Windows card", () => {
    expect(pickDownloads([release("0.1.0", ["foxxycode_0.1.0_linux_amd64.tar.gz"])]).cliWindows).toBeNull();
  });
});

describe("fetchReleases", () => {
  it("sends the token and retries a failed request", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 502, statusText: "Bad Gateway" })
      .mockResolvedValueOnce({ ok: true, json: async () => [release("0.1.0", [])] });
    vi.useFakeTimers();
    const pending = fetchReleases("o/r", { token: "t0k", fetchImpl: fetchImpl as unknown as typeof fetch });
    await vi.runAllTimersAsync();
    const releases = await pending;
    vi.useRealTimers();
    expect(releases).toHaveLength(1);
    expect(fetchImpl).toHaveBeenCalledTimes(2);
    const [url, init] = fetchImpl.mock.calls[0]!;
    expect(url).toBe("https://api.github.com/repos/o/r/releases?per_page=50");
    expect(init.headers.Authorization).toBe("Bearer t0k");
  });
});
