import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import { Settings } from "./Settings";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
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
