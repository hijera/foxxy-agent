import React from "react";
import { afterEach } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { BranchNavigator } from "./BranchNavigator";

afterEach(() => cleanup());

const SESSIONS = [
  { sessionId: "sess_a" },
  { sessionId: "sess_b" },
  { sessionId: "sess_c" },
];

test("renders label with 1-based current/total", () => {
  render(
    <BranchNavigator
      userMessageIndex={0}
      currentIndex={1}
      total={3}
      sessions={SESSIONS}
      onSwitch={vi.fn()}
    />,
  );
  expect(screen.getByTestId("branch-nav-label")).toHaveTextContent("2/3");
});

test("prev button disabled at first branch", () => {
  render(
    <BranchNavigator
      userMessageIndex={0}
      currentIndex={0}
      total={3}
      sessions={SESSIONS}
      onSwitch={vi.fn()}
    />,
  );
  expect(screen.getByTestId("branch-nav-prev")).toBeDisabled();
  expect(screen.getByTestId("branch-nav-next")).not.toBeDisabled();
});

test("next button disabled at last branch", () => {
  render(
    <BranchNavigator
      userMessageIndex={0}
      currentIndex={2}
      total={3}
      sessions={SESSIONS}
      onSwitch={vi.fn()}
    />,
  );
  expect(screen.getByTestId("branch-nav-next")).toBeDisabled();
  expect(screen.getByTestId("branch-nav-prev")).not.toBeDisabled();
});

test("clicking prev calls onSwitch with previous session id", () => {
  const onSwitch = vi.fn();
  render(
    <BranchNavigator
      userMessageIndex={0}
      currentIndex={1}
      total={3}
      sessions={SESSIONS}
      onSwitch={onSwitch}
    />,
  );
  screen.getByTestId("branch-nav-prev").click();
  expect(onSwitch).toHaveBeenCalledWith("sess_a");
});

test("clicking next calls onSwitch with next session id", () => {
  const onSwitch = vi.fn();
  render(
    <BranchNavigator
      userMessageIndex={0}
      currentIndex={1}
      total={3}
      sessions={SESSIONS}
      onSwitch={onSwitch}
    />,
  );
  screen.getByTestId("branch-nav-next").click();
  expect(onSwitch).toHaveBeenCalledWith("sess_c");
});
