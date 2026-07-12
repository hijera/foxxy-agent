import { describe, expect, it } from "vitest";

import {
  isBrowserToolName,
  parseBrowserActionResult,
  sessionAssetUrl,
} from "./browserActionDisplay";

describe("isBrowserToolName", () => {
  it("matches foxxycode_browser_* tools", () => {
    expect(isBrowserToolName("foxxycode_browser_navigate")).toBe(true);
    expect(isBrowserToolName("foxxycode_browser_click")).toBe(true);
    expect(isBrowserToolName("apply_patch")).toBe(false);
    expect(isBrowserToolName(undefined)).toBe(false);
  });
});

describe("parseBrowserActionResult", () => {
  it("returns null for empty input", () => {
    expect(parseBrowserActionResult("")).toBeNull();
    expect(parseBrowserActionResult(undefined)).toBeNull();
  });

  it("parses action, url, screenshot basename and console (unix path)", () => {
    const res = [
      "navigated to https://example.com",
      "url: https://example.com/",
      "screenshot: /home/u/.foxxycode/sessions/s1/assets/browser_123.png",
      "console:",
      "  [log] hello",
      "  [error] boom",
    ].join("\n");
    const info = parseBrowserActionResult(res);
    expect(info).not.toBeNull();
    expect(info!.action).toBe("navigated to https://example.com");
    expect(info!.url).toBe("https://example.com/");
    expect(info!.screenshotName).toBe("browser_123.png");
    expect(info!.console).toEqual(["[log] hello", "[error] boom"]);
  });

  it("extracts basename from a windows path", () => {
    const res = [
      "captured screenshot",
      "screenshot: H:\\proj\\.foxxycode\\sessions\\s1\\assets\\browser_9.png",
    ].join("\n");
    const info = parseBrowserActionResult(res);
    expect(info!.screenshotName).toBe("browser_9.png");
  });

  it("omits screenshot when unavailable", () => {
    const res = ["clicked #go", "screenshot: unavailable (timeout)"].join("\n");
    const info = parseBrowserActionResult(res);
    expect(info!.screenshotName).toBeUndefined();
  });

  it("keeps the action line for error results", () => {
    const info = parseBrowserActionResult("error: navigate https://x: boom");
    expect(info!.action).toBe("error: navigate https://x: boom");
    expect(info!.screenshotName).toBeUndefined();
  });
});

describe("sessionAssetUrl", () => {
  it("builds an encoded assets URL", () => {
    expect(sessionAssetUrl("s 1", "browser_1.png")).toBe(
      "/foxxycode/sessions/s%201/assets/browser_1.png",
    );
  });
});
