import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { OPEN_EXTERNAL_MESSAGE_TYPE, externalUrlFromMessage } from "../src/webview/externalLink";

const REPO = join(__dirname, "..", "..", "..");
const read = (...parts: string[]) => readFileSync(join(REPO, ...parts), "utf8").replace(/\r\n/g, "\n");

describe("externalUrlFromMessage", () => {
  it("accepts http and https links from the SPA", () => {
    expect(externalUrlFromMessage({ type: OPEN_EXTERNAL_MESSAGE_TYPE, url: "https://hijera.github.io/foxxy-agent/" })).toBe(
      "https://hijera.github.io/foxxy-agent/",
    );
    expect(externalUrlFromMessage({ type: OPEN_EXTERNAL_MESSAGE_TYPE, url: "http://127.0.0.1:40123/docs/" })).toBe(
      "http://127.0.0.1:40123/docs/",
    );
  });

  it("refuses other schemes, other messages and malformed input", () => {
    for (const url of ["file:///C:/Windows/System32/calc.exe", "command:workbench.action.quit", "vscode://settings", "javascript:alert(1)", "not a url"]) {
      expect(externalUrlFromMessage({ type: OPEN_EXTERNAL_MESSAGE_TYPE, url }), url).toBeNull();
    }
    expect(externalUrlFromMessage({ type: "foxxycode:locale", url: "https://example.com/" })).toBeNull();
    expect(externalUrlFromMessage({ type: OPEN_EXTERNAL_MESSAGE_TYPE, url: 42 })).toBeNull();
    expect(externalUrlFromMessage(null)).toBeNull();
  });
});

describe("the open-external relay", () => {
  it("uses the same message type in the SPA, the webview wrapper and the extension host", () => {
    const spa = read("external", "ui", "src", "ui", "embedExternalLinks.ts");
    expect(spa).toContain(`export const OPEN_EXTERNAL_MESSAGE_TYPE = "${OPEN_EXTERNAL_MESSAGE_TYPE}";`);
    const panel = read("editors", "vscode", "src", "webview", "panel.ts");
    // The wrapper script forwards the frame's message to the extension host...
    expect(panel).toContain('d.type === "${OPEN_EXTERNAL_MESSAGE_TYPE}"');
    // ...and the host opens it through the checked URL only.
    expect(panel).toMatch(/externalUrlFromMessage\(msg\)[\s\S]{0,200}vscode\.env\.openExternal/);
  });
});
