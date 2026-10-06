import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import { Settings } from "./Settings";

afterEach(() => {
  cleanup();
  // unstubAllGlobals, not restoreAllMocks: the latter also wipes the matchMedia mock that
  // vitest.setup.ts installs once, and every later render in this file then crashes.
  vi.unstubAllGlobals();
});

function mockConfigFetch(ok = true) {
  const fetchMock = vi.fn().mockImplementation(async (path: string) => {
    if (path === "/foxxycode/config/schema") {
      const body = ok
        ? { type: "object", properties: {} }
        : { type: "object", properties: {} };
      return {
        ok,
        status: ok ? 200 : 500,
        json: async () => body,
      } as unknown as Response;
    }
    if (path === "/foxxycode/config") {
      return {
        ok,
        status: ok ? 200 : 500,
        json: async () => ({}),
      } as unknown as Response;
    }
    return { ok: false, status: 404, json: async () => ({}) } as unknown as Response;
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

test("settings footer exposes the API docs link (moved out of the hero footer)", async () => {
  mockConfigFetch(true);
  const { getByTestId } = render(<Settings onClose={() => {}} />);

  const link = await waitFor(() => getByTestId("settings-api-docs-link"));
  expect(link.getAttribute("href")).toBe("/docs/");
  expect(link.tagName).toBe("A");
  expect(link.textContent).toContain("API docs");
});

test("settings footer links to the project website next to the API docs", async () => {
  mockConfigFetch(true);
  const { getByTestId } = render(<Settings onClose={() => {}} />);

  const site = await waitFor(() => getByTestId("settings-site-link"));
  expect(site.tagName).toBe("A");
  expect(site.getAttribute("href")).toBe("https://hijera.github.io/foxxy-agent/");
  expect(site.getAttribute("target")).toBe("_blank");
  expect(site.getAttribute("rel")).toContain("noopener");
  expect(site.textContent).toContain("Website");
  // Same footer row, right after the API docs link.
  expect(getByTestId("settings-api-docs-link").nextElementSibling).toBe(site);
});
