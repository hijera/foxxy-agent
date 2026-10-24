import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ContextBreakdownPopover } from "./ContextBreakdownPopover";
import { OpenRailScreen } from "../nav/railEscape.fakes";

afterEach(cleanup);

// History open beside the chat, the breakdown opened over the composer after
// it: Escape takes the breakdown down first, and History stays for the next one.
test("Escape closes the breakdown before the drawer open beside the chat", () => {
  const onClose = vi.fn();
  const closeHistory = vi.fn();
  render(
    <>
      <OpenRailScreen id="history" onClose={closeHistory} />
      <ContextBreakdownPopover open onClose={onClose} maxContextTokens={128000} />
    </>,
  );
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(onClose).toHaveBeenCalledTimes(1);
  expect(closeHistory).not.toHaveBeenCalled();
});
