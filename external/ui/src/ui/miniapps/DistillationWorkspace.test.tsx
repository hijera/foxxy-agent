import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { DistillationWorkspace } from "./DistillationWorkspace";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const candidates = [
  {
    id: "scenario-1",
    task: "Write a greeting",
    accepted_outcome: "The greeting is written",
  },
];

test("cancelling scenario review leaves the waiting state", async () => {
  let cancelled = false;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = String(input);
    if (path.includes("/events")) {
      return new Response("unavailable", { status: 500 });
    }
    if (path.endsWith("/cancel")) {
      cancelled = true;
    }
    if (path.endsWith("/miniapps/distill") || path.endsWith("/cancel")) {
      return new Response(
        JSON.stringify({
          id: "distill-job",
          status: cancelled ? "cancelled" : "waiting_for_scenario",
          phase: cancelled ? "cancelled" : "scenario_review",
          candidates,
        }),
        { status: cancelled ? 200 : 202 },
      );
    }
    return new Response(
      JSON.stringify({
        id: "distill-job",
        status: cancelled ? "cancelled" : "waiting_for_scenario",
        phase: cancelled ? "cancelled" : "scenario_review",
        candidates,
      }),
    );
  });

  render(
    <DistillationWorkspace
      sessionId="session-1"
      onComplete={() => {}}
      onCancel={() => {}}
    />,
  );
  await screen.findByRole("button", { name: "Generate workflow" });
  fireEvent.click(screen.getAllByRole("button", { name: "Cancel" })[0]!);

  await waitFor(() => expect(cancelled).toBe(true));
  expect(await screen.findByText("cancelled")).toBeInTheDocument();
  expect(screen.queryByRole("radio")).toBeNull();
});

test("a failed synthesis shows its error instead of reopening scenario review", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = String(input);
    if (path.includes("/events")) {
      return new Response("unavailable", { status: 500 });
    }
    return new Response(
      JSON.stringify({
        id: "distill-job",
        status: "failed",
        phase: "failed",
        error: "profile generation failed",
        candidates,
      }),
      { status: 202 },
    );
  });

  render(
    <DistillationWorkspace
      sessionId="session-1"
      onComplete={() => {}}
      onCancel={() => {}}
    />,
  );

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "profile generation failed",
  );
  expect(screen.queryByRole("radio")).toBeNull();
});
