import { expect, test, vi } from "vitest";
import { sameTabInAppNavClick } from "./sameTabInAppNav";

function makeMouseEvent(
  init: Partial<{
    button: number;
    metaKey: boolean;
    ctrlKey: boolean;
    shiftKey: boolean;
    altKey: boolean;
    defaultPrevented: boolean;
  }>,
): Parameters<typeof sameTabInAppNavClick>[0] {
  return {
    button: init.button ?? 0,
    metaKey: init.metaKey ?? false,
    ctrlKey: init.ctrlKey ?? false,
    shiftKey: init.shiftKey ?? false,
    altKey: init.altKey ?? false,
    defaultPrevented: init.defaultPrevented ?? false,
    preventDefault: vi.fn(),
  } as unknown as Parameters<typeof sameTabInAppNavClick>[0];
}

test("plain left click runs action and prevents default", () => {
  const action = vi.fn();
  const ev = makeMouseEvent({});
  sameTabInAppNavClick(ev, action);
  expect(action).toHaveBeenCalledTimes(1);
  expect(ev.preventDefault).toHaveBeenCalled();
});

test("middle button does not run action", () => {
  const action = vi.fn();
  const ev = makeMouseEvent({ button: 1 });
  sameTabInAppNavClick(ev, action);
  expect(action).not.toHaveBeenCalled();
  expect(ev.preventDefault).not.toHaveBeenCalled();
});

test("modified left click does not run action", () => {
  const action = vi.fn();
  const ev = makeMouseEvent({ ctrlKey: true });
  sameTabInAppNavClick(ev, action);
  expect(action).not.toHaveBeenCalled();
  expect(ev.preventDefault).not.toHaveBeenCalled();
});
