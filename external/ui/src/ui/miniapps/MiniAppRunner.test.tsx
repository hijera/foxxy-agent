import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
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

test("a successful history entry shows completed progress without live events", () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response("unavailable", { status: 404 }),
  );
  render(
    <MiniAppRunner
      app={app}
      released={false}
      initialRun={{ id: "persisted-run", status: "succeeded" }}
      onClose={() => {}}
    />,
  );
  expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");
});

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

test("integer inputs are submitted as JSON numbers", async () => {
  let submitted: { inputs?: Record<string, unknown> } | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input);
    if (path.endsWith("/test-runs")) {
      submitted = JSON.parse(String(init?.body)) as typeof submitted;
      return new Response(
        JSON.stringify({ id: "integer-job", status: "queued" }),
        { status: 202 },
      );
    }
    if (path.includes("/events")) {
      return new Response("unavailable", { status: 500 });
    }
    return new Response(
      JSON.stringify({ id: "integer-job", status: "failed" }),
    );
  });
  const integerApp: MiniAppDocument = {
    ...app,
    inputs: [
      {
        id: "count",
        type: "integer",
        title: "Count",
        required: true,
        ui: { control: "number" },
      },
    ],
  };

  render(
    <MiniAppRunner app={integerApp} released={false} onClose={() => {}} />,
  );
  fireEvent.change(screen.getByLabelText("Count"), { target: { value: "7" } });
  fireEvent.click(screen.getByTestId("miniapps-run-start"));

  await waitFor(() => expect(submitted).toBeDefined());
  expect(submitted).toEqual({ inputs: { count: 7 } });
});

test("a declared file input provides an upload control and declarative submit label", () => {
  const fileApp: MiniAppDocument = {
    ...app,
    display: { title: "Audio extractor", submit_label: "Extract MP3" },
    inputs: [
      {
        id: "source",
        title: "Video",
        type: "file",
        required: true,
        ui: { control: "file" },
      },
    ],
  };
  render(<MiniAppRunner app={fileApp} released={false} onClose={() => {}} />);
  expect(screen.getByLabelText("Video")).toHaveAttribute("type", "file");
  expect(screen.getByTestId("miniapps-run-start")).toHaveTextContent(
    "Extract MP3",
  );
});

test("a terminal snapshot clears the old confirmation and refreshes history", async () => {
  const completed = vi.fn();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) =>
    String(input).includes("/events")
      ? new Response("", { status: 500 })
      : new Response(
          JSON.stringify({
            id: "finished-job",
            app_id: app.id,
            status: "failed",
            error: "No audio track",
          }),
        ),
  );
  render(
    <MiniAppRunner
      app={app}
      released={false}
      initialRun={{
        id: "finished-job",
        status: "waiting_for_confirmation",
        confirmation: { id: "trust", message: "Trust encoder?" },
      }}
      onCompleted={completed}
      onClose={() => {}}
    />,
  );
  expect(await screen.findByText("No audio track")).toBeInTheDocument();
  expect(screen.queryByText("Trust encoder?")).toBeNull();
  await waitFor(() =>
    expect(completed).toHaveBeenCalledWith(
      expect.objectContaining({ status: "failed" }),
    ),
  );
});
