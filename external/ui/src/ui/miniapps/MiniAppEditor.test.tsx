import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MiniAppEditor } from "./MiniAppEditor";
import type { MiniAppDocument } from "./types";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

test("editing a displayed expected result updates the enforced field", async () => {
  let submitted: MiniAppDocument | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    submitted = JSON.parse(String(init?.body)) as MiniAppDocument;
    return new Response(
      JSON.stringify({ ...submitted, revision: "revision-2" }),
    );
  });
  const initial: MiniAppDocument = {
    schema_version: "1.0.0",
    kind: "foxxycode.miniapp",
    id: "expected-result-app",
    state: "draft",
    revision: "revision-1",
    metadata: { name: "Expected result", goal: "Check the result" },
    success: {
      mode: "all",
      expected_result: "before",
      checks: [],
    },
  };

  render(<MiniAppEditor initial={initial} onRun={() => {}} />);
  fireEvent.change(screen.getByDisplayValue("before"), {
    target: { value: "after" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save draft" }));

  await waitFor(() => expect(submitted).toBeDefined());
  expect(submitted?.success?.expected_result).toBe("after");
  expect(submitted?.success?.expectations).toBeUndefined();
});

test("applying an assistant proposal replaces the document instead of merging it", async () => {
  let submitted: MiniAppDocument | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/assistant")) {
      return new Response(
        JSON.stringify({
          reply: "Dropped the extra input.",
          draft: {
            schema_version: "1.0.0",
            kind: "foxxycode.miniapp",
            id: "proposal-app",
            state: "draft",
            revision: "revision-9",
            metadata: { name: "Proposal", goal: "Check replace" },
            success: { mode: "all", checks: [] },
          },
        }),
      );
    }
    submitted = JSON.parse(String(init?.body)) as MiniAppDocument;
    return new Response(JSON.stringify({ ...submitted, revision: "revision-3" }));
  });
  const initial: MiniAppDocument = {
    schema_version: "1.0.0",
    kind: "foxxycode.miniapp",
    id: "proposal-app",
    state: "draft",
    revision: "revision-1",
    metadata: { name: "Proposal", goal: "Check replace" },
    inputs: [{ id: "extra", type: "string", title: "Extra" }],
    success: { mode: "all", checks: [] },
  };

  render(<MiniAppEditor initial={initial} onRun={() => {}} />);
  fireEvent.change(screen.getByLabelText("Message to the Mini App assistant"), {
    target: { value: "drop the extra input" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Ask agent" }));
  fireEvent.click(await screen.findByRole("button", { name: "Apply changes" }));
  fireEvent.click(screen.getByRole("button", { name: "Save draft" }));

  await waitFor(() => expect(submitted).toBeDefined());
  // A proposal is a complete document: a field it dropped must not survive
  // the merge from the current draft.
  expect(submitted?.inputs).toBeUndefined();
});

