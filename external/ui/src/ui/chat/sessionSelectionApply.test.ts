import { expect, test } from "vitest";
import {
  shouldAdoptServerModeAfterTurn,
  shouldApplySessionSelection,
} from "./sessionSelectionApply";

const base = {
  stashSid: "sess_a",
  viewedSid: "sess_a",
  appliedSid: "",
  userTouchedSid: "",
};

test("opening a session applies its stored selection", () => {
  expect(shouldApplySessionSelection(base)).toBe(true);
});

test("a second messages load for the same session does not re-apply it", () => {
  expect(
    shouldApplySessionSelection({ ...base, appliedSid: "sess_a" }),
  ).toBe(false);
});

test("opening a different session applies that session's selection", () => {
  expect(
    shouldApplySessionSelection({
      stashSid: "sess_b",
      viewedSid: "sess_b",
      appliedSid: "sess_a",
      userTouchedSid: "sess_a",
    }),
  ).toBe(true);
});

test("a snapshot that lands after the viewer moved on is dropped", () => {
  expect(
    shouldApplySessionSelection({ ...base, viewedSid: "sess_b" }),
  ).toBe(false);
});

test("a pick the user already made outranks the first snapshot", () => {
  expect(
    shouldApplySessionSelection({ ...base, userTouchedSid: "sess_a" }),
  ).toBe(false);
});

test("an empty session id applies nothing", () => {
  expect(
    shouldApplySessionSelection({ ...base, stashSid: "", viewedSid: "" }),
  ).toBe(false);
});

const modes = ["agent", "plan", "docs", "ask", "debug"] as const;

const afterTurn = {
  serverMode: "agent",
  currentMode: "plan",
  knownModes: modes,
  editSeqAtTurnStart: 3,
  editSeqNow: 3,
};

test("a server-side switch missed on the stream is adopted after the turn", () => {
  expect(shouldAdoptServerModeAfterTurn(afterTurn)).toBe(true);
});

test("a mode the user switched during the turn is not overwritten", () => {
  expect(
    shouldAdoptServerModeAfterTurn({ ...afterTurn, editSeqNow: 4 }),
  ).toBe(false);
});

test("a mode that already matches needs no adoption", () => {
  expect(
    shouldAdoptServerModeAfterTurn({ ...afterTurn, currentMode: "agent" }),
  ).toBe(false);
});

test("an unknown or empty mode from the server is ignored", () => {
  expect(
    shouldAdoptServerModeAfterTurn({ ...afterTurn, serverMode: "wizard" }),
  ).toBe(false);
  expect(
    shouldAdoptServerModeAfterTurn({ ...afterTurn, serverMode: "" }),
  ).toBe(false);
});
