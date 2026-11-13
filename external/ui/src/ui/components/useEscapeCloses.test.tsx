import React from "react";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { useEscapeCloses } from "./useEscapeCloses";
import { OpenRailScreen } from "../nav/railEscape.fakes";

afterEach(cleanup);

function Layer(props: { open: boolean; close: () => void }) {
  useEscapeCloses(props.open, props.close);
  return null;
}

test("Escape closes an open layer and claims the key", () => {
  const close = vi.fn();
  render(<Layer open close={close} />);
  expect(fireEvent.keyDown(document.body, { key: "Escape" })).toBe(false);
  expect(close).toHaveBeenCalledTimes(1);
});

// The screen listens from before the layer opened; the layer hears the key
// first all the same, and the screen stays for the next Escape.
test("the screen of the rail under a layer stays for the next Escape", () => {
  const closeLayer = vi.fn();
  const closeScreen = vi.fn();
  const { rerender } = render(
    <>
      <OpenRailScreen id="history" onClose={closeScreen} />
      <Layer open={false} close={closeLayer} />
    </>,
  );
  rerender(
    <>
      <OpenRailScreen id="history" onClose={closeScreen} />
      <Layer open close={closeLayer} />
    </>,
  );
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(closeLayer).toHaveBeenCalledTimes(1);
  expect(closeScreen).not.toHaveBeenCalled();
  rerender(
    <>
      <OpenRailScreen id="history" onClose={closeScreen} />
      <Layer open={false} close={closeLayer} />
    </>,
  );
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(closeScreen).toHaveBeenCalledTimes(1);
});

test("a closed layer, a claimed Escape and a composing one close nothing", () => {
  const close = vi.fn();
  const { rerender } = render(<Layer open={false} close={close} />);
  expect(fireEvent.keyDown(document.body, { key: "Escape" })).toBe(true);
  rerender(<Layer open close={close} />);
  const claim = (e: KeyboardEvent) => e.preventDefault();
  window.addEventListener("keydown", claim, true);
  try {
    fireEvent.keyDown(document.body, { key: "Escape" });
  } finally {
    window.removeEventListener("keydown", claim, true);
  }
  fireEvent.keyDown(document.body, { key: "Escape", isComposing: true });
  fireEvent.keyDown(document.body, { key: "Enter" });
  expect(close).not.toHaveBeenCalled();
});
