import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { CommandsPanel } from "./CommandsPanel";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const untrustedRow = {
  name: "fakeenc_convert",
  binary: "fakeenc",
  permission: "allow",
  hash: "abc123",
  resolved_path: "C:/tools/fakeenc.exe",
  installed: true,
  trusted: false,
  source: "document",
};

test("an installed but untrusted profile offers the trust action and records it", async () => {
  const calls: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    calls.push(`${init?.method ?? "GET"} ${url}`);
    if (url.endsWith("/trust")) return json({ ...untrustedRow, trusted: true });
    if (calls.filter((c) => c.startsWith("GET")).length > 1)
      return json({ items: [{ ...untrustedRow, trusted: true }] });
    return json({ items: [untrustedRow] });
  });

  render(<CommandsPanel appId="demo-app" />);
  expect(await screen.findByText("fakeenc_convert")).toBeInTheDocument();
  expect(screen.getByText("Not trusted")).toBeInTheDocument();

  fireEvent.click(screen.getByTestId("miniapps-command-trust-fakeenc_convert"));
  await waitFor(() =>
    expect(
      calls.some((c) =>
        c.includes("/foxxycode/miniapps/demo-app/commands/fakeenc_convert/trust"),
      ),
    ).toBe(true),
  );
  expect(await screen.findByText("Trusted")).toBeInTheDocument();
});

test("a missing binary offers the detected package managers with the exact command", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    json({
      items: [
        {
          ...untrustedRow,
          resolved_path: undefined,
          installed: false,
          managers: [
            {
              id: "winget",
              package: "Fake.Enc",
              command: "winget install --id Fake.Enc -e",
            },
          ],
        },
      ],
    }),
  );
  render(<CommandsPanel appId="demo-app" />);
  const install = await screen.findByTestId(
    "miniapps-command-install-fakeenc_convert-winget",
  );
  expect(install).toHaveAttribute("title", "winget install --id Fake.Enc -e");
  expect(screen.getByText("Not installed")).toBeInTheDocument();
  // Nothing to trust yet: the approval binds to a resolved path.
  expect(
    screen.queryByTestId("miniapps-command-trust-fakeenc_convert"),
  ).toBeNull();
});

test("the panel renders nothing for an app without command profiles", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(json({ items: [] }));
  const { container } = render(<CommandsPanel appId="demo-app" />);
  await waitFor(() => expect(container.firstChild).toBeNull());
});
