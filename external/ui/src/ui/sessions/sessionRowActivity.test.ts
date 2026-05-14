import { expect, test } from "vitest";
import {
  sessionRowShowsSpinner,
  sessionRowShowsUnreadDot,
} from "./sessionRowActivity";
import type { SessionRow } from "./types";

const base = (id: string, o: Partial<SessionRow> = {}): SessionRow => ({
  id,
  title: "t",
  ...o,
});

test("spinner when another session has active turn", () => {
  expect(
    sessionRowShowsSpinner(base("a", { turnActive: true }), "b"),
  ).toBe(true);
  expect(
    sessionRowShowsSpinner(base("a", { turnActive: true }), "a"),
  ).toBe(false);
  expect(sessionRowShowsSpinner(base("a", { turnActive: false }), "b")).toBe(
    false,
  );
});

test("unread dot when another session has unread completion", () => {
  expect(
    sessionRowShowsUnreadDot(base("a", { unreadComplete: true }), "b"),
  ).toBe(true);
  expect(
    sessionRowShowsUnreadDot(base("a", { unreadComplete: true }), "a"),
  ).toBe(false);
});

