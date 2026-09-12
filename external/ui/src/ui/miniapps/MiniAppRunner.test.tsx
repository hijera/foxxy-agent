import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MiniAppRunner } from "./MiniAppRunner";
import type { MiniAppDocument } from "./types";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const app: MiniAppDocument = {
  schema_version: "1.0.0",
  kind: "foxxycode.miniapp",
  id: "greeting-app",
  state: "draft",
  metadata: { name: "Greeting", goal: "Write a greeting" },
};

// A start that never produces a run leaves runId empty, so anything rendered
// only in the running branch stays invisible: the operator pressed the button
// and saw nothing happen.
test("a failed start reports the error instead of doing nothing visible", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(
      JSON.stringify({
        error: { code: "not_found", message: "source evidence is missing" },
      }),
      { status: 404, headers: { "Content-Type": "application/json" } },
    ),
  );

  render(<MiniAppRunner app={app} released={false} onClose={() => {}} />);
  fireEvent.click(screen.getByTestId("miniapps-run-start"));

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "source evidence is missing",
  );
});
