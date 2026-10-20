import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import {
  OPEN_EXTERNAL_MESSAGE_TYPE,
  externalLinkUrl,
  installEmbedExternalLinks,
} from "./embedExternalLinks";

function anchor(attrs: Record<string, string>, text = "link"): HTMLAnchorElement {
  const a = document.createElement("a");
  for (const [k, v] of Object.entries(attrs)) a.setAttribute(k, v);
  a.textContent = text;
  document.body.appendChild(a);
  return a;
}

describe("externalLinkUrl", () => {
  test("resolves new-tab http(s) links, relative ones included", () => {
    expect(externalLinkUrl(anchor({ href: "https://hijera.github.io/foxxy-agent/", target: "_blank" }))).toBe(
      "https://hijera.github.io/foxxy-agent/",
    );
    expect(externalLinkUrl(anchor({ href: "/docs/", target: "_blank" }))).toBe(`${window.location.origin}/docs/`);
  });

  test("leaves same-tab, download and non-http links to the page", () => {
    expect(externalLinkUrl(anchor({ href: "https://example.com/" }))).toBeNull();
    expect(externalLinkUrl(anchor({ href: "https://example.com/f.pdf", target: "_blank", download: "" }))).toBeNull();
    expect(externalLinkUrl(anchor({ href: "mailto:a@b.c", target: "_blank" }))).toBeNull();
    expect(externalLinkUrl(anchor({ href: "javascript:void(0)", target: "_blank" }))).toBeNull();
  });
});

describe("installEmbedExternalLinks", () => {
  let dispose: (() => void) | null = null;
  const postMessage = vi.fn();

  beforeEach(() => {
    window.history.replaceState(null, "", "/?embed=vscode");
    vi.spyOn(window, "parent", "get").mockReturnValue({ postMessage } as unknown as Window);
  });

  afterEach(() => {
    dispose?.();
    dispose = null;
    postMessage.mockReset();
    vi.restoreAllMocks();
    document.body.innerHTML = "";
    window.history.replaceState(null, "", "/");
    window.sessionStorage.clear();
  });

  test("in the VS Code embed a new-tab link asks the host to open the system browser", () => {
    dispose = installEmbedExternalLinks();
    const a = anchor({ href: "https://hijera.github.io/foxxy-agent/", target: "_blank" });
    const span = document.createElement("span");
    a.appendChild(span);
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
    span.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    expect(postMessage).toHaveBeenCalledWith(
      { type: OPEN_EXTERNAL_MESSAGE_TYPE, url: "https://hijera.github.io/foxxy-agent/" },
      "*",
    );
  });

  test("ordinary in-app links keep working", () => {
    dispose = installEmbedExternalLinks();
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
    anchor({ href: "#/settings" }).dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(false);
    expect(postMessage).not.toHaveBeenCalled();
  });

  test("stays off outside the VS Code embed", () => {
    window.history.replaceState(null, "", "/?embed=intellij");
    expect(installEmbedExternalLinks()).toBeNull();
    window.history.replaceState(null, "", "/");
    expect(installEmbedExternalLinks()).toBeNull();
  });
});
