import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { GuidedTour } from "./GuidedTour";
import type { TourStep } from "./tourSteps";

const steps: TourStep[] = [
  { id: "a", anchor: "#anchor-a", titleKey: "x.a.title", bodyKey: "x.a.body" },
  {
    id: "b",
    anchor: "#anchor-b-missing",
    titleKey: "x.b.title",
    bodyKey: "x.b.body",
    optional: true,
  },
  { id: "c", anchor: "#anchor-c", titleKey: "x.c.title", bodyKey: "x.c.body" },
];

beforeEach(() => {
  const a = document.createElement("div");
  a.id = "anchor-a";
  const c = document.createElement("div");
  c.id = "anchor-c";
  document.body.append(a, c);
});

afterEach(() => {
  cleanup();
  document.getElementById("anchor-a")?.remove();
  document.getElementById("anchor-c")?.remove();
});

test("skips steps whose anchor is absent and counts only present ones", () => {
  render(<GuidedTour open steps={steps} onClose={() => {}} />);
  // Two of three anchors exist (#anchor-b-missing is filtered out).
  expect(screen.getByTestId("tour-counter").textContent).toBe("1 of 2");
});

test("Next / Back navigate and the last step finishes the tour", () => {
  const onClose = vi.fn();
  render(<GuidedTour open steps={steps} onClose={onClose} />);

  // Step 1: no Back button yet.
  expect(screen.queryByTestId("tour-back")).toBeNull();

  fireEvent.click(screen.getByTestId("tour-next"));
  expect(screen.getByTestId("tour-counter").textContent).toBe("2 of 2");
  // Last step: primary button reads "Done".
  expect(screen.getByTestId("tour-next").textContent).toBe("Done");

  fireEvent.click(screen.getByTestId("tour-back"));
  expect(screen.getByTestId("tour-counter").textContent).toBe("1 of 2");

  fireEvent.click(screen.getByTestId("tour-next"));
  fireEvent.click(screen.getByTestId("tour-next"));
  expect(onClose).toHaveBeenCalledTimes(1);
});

test("Skip closes the tour", () => {
  const onClose = vi.fn();
  render(<GuidedTour open steps={steps} onClose={onClose} />);
  fireEvent.click(screen.getByTestId("tour-skip"));
  expect(onClose).toHaveBeenCalledTimes(1);
});

test("closes immediately when no anchors are present", () => {
  const onClose = vi.fn();
  render(
    <GuidedTour
      open
      steps={[
        { id: "z", anchor: "#nope", titleKey: "x.z.title", bodyKey: "x.z.body" },
      ]}
      onClose={onClose}
    />,
  );
  expect(onClose).toHaveBeenCalledTimes(1);
  expect(screen.queryByTestId("guided-tour")).toBeNull();
});
