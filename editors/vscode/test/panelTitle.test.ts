import { describe, it, expect } from "vitest";
import {
  formatPanelTitle,
  formatViewDescription,
  PANEL_PRODUCT_NAME,
} from "../src/webview/panelTitle";

describe("panel chrome naming", () => {
  it("puts the version after the product name in the editor tab title", () => {
    expect(formatPanelTitle("0.1.6")).toBe("FoxxyCode 0.1.6");
  });

  it("falls back to the bare product name without a version", () => {
    expect(formatPanelTitle(undefined)).toBe(PANEL_PRODUCT_NAME);
    expect(formatPanelTitle(null)).toBe(PANEL_PRODUCT_NAME);
    expect(formatPanelTitle("   ")).toBe(PANEL_PRODUCT_NAME);
  });

  it("view description is the bare version, since VS Code renders it after the name", () => {
    expect(formatViewDescription("0.1.6")).toBe("0.1.6");
    expect(formatViewDescription(undefined)).toBe("");
  });
});
