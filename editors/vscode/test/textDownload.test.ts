import { describe, expect, it } from "vitest";
import { DOWNLOAD_TEXT_MESSAGE_TYPE, textDownloadFromMessage } from "../src/webview/textDownload";

describe("diagram download messages", () => {
  it("preserves Unicode and whitespace in each supported format", () => {
    for (const filename of ["diagram.mmd", "diagram.svg", "image.svg"]) {
      const message = { type: DOWNLOAD_TEXT_MESSAGE_TYPE, filename, text: "Привет 世界\n  A --> B\n" };
      expect(textDownloadFromMessage(message)).toEqual({ filename, text: message.text });
    }
  });

  it("rejects paths, unsupported formats and malformed messages", () => {
    for (const filename of ["../diagram.svg", "C:\\diagram.svg", "diagram.exe", "diagram.svg/other", ""]) {
      expect(textDownloadFromMessage({ type: DOWNLOAD_TEXT_MESSAGE_TYPE, filename, text: "source" })).toBeNull();
    }
    for (const message of [null, "text", {}, { type: "other", filename: "image.svg", text: "svg" },
      { type: DOWNLOAD_TEXT_MESSAGE_TYPE, filename: "image.svg", text: 42 }]) {
      expect(textDownloadFromMessage(message)).toBeNull();
    }
  });
});
