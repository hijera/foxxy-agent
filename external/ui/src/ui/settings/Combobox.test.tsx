import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { Combobox } from "./Combobox";
import { OpenRailScreen } from "../nav/railEscape.fakes";

afterEach(cleanup);

function Harness({ initial = "" }: { initial?: string }) {
  const [v, setV] = React.useState(initial);
  return (
    <>
      <Combobox
        value={v}
        onChange={setV}
        options={[{ value: "openai" }, { value: "anthropic" }]}
        ariaLabel="Type"
        testid="cb"
      />
      <span data-testid="val">{v}</span>
    </>
  );
}

test("shows all options on focus and picks one", () => {
  render(<Harness />);
  fireEvent.focus(screen.getByTestId("cb"));
  expect(screen.getByText("anthropic")).toBeTruthy();
  fireEvent.mouseDown(screen.getByText("anthropic"));
  expect(screen.getByTestId("val").textContent).toBe("anthropic");
});

test("shows the option label even when a mouse or keyboard selection keeps focus", () => {
  function LabeledHarness() {
    const [value, setValue] = React.useState("keep");
    return (
      <>
        <Combobox
          value={value}
          onChange={setValue}
          options={[
            { value: "keep", label: "Keep screenshots" },
            { value: "latest", label: "Remove intermediate screenshots" },
          ]}
          showOptionLabel
          ariaLabel="Screenshot mode"
        />
        <span data-testid="mode-key">{value}</span>
      </>
    );
  }

  render(<LabeledHarness />);
  const input = screen.getByRole<HTMLInputElement>("combobox");
  act(() => input.focus());
  fireEvent.mouseDown(
    screen.getByRole("option", { name: "Remove intermediate screenshots" }),
  );
  expect(document.activeElement).toBe(input);
  expect(input.value).toBe("Remove intermediate screenshots");
  expect(screen.getByTestId("mode-key").textContent).toBe("latest");

  fireEvent.keyDown(input, { key: "ArrowDown" });
  fireEvent.keyDown(input, { key: "ArrowDown" });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(input.value).toBe("Keep screenshots");
  expect(screen.getByTestId("mode-key").textContent).toBe("keep");
});

test("typing filters options and keeps the typed text", () => {
  render(<Harness />);
  fireEvent.change(screen.getByTestId("cb"), { target: { value: "anth" } });
  expect(screen.getByText("anthropic")).toBeTruthy();
  expect(screen.queryByText("openai")).toBeNull();
  expect(screen.getByTestId("val").textContent).toBe("anth");
});

test("accepts a free-text value not in the options", () => {
  render(<Harness />);
  fireEvent.change(screen.getByTestId("cb"), { target: { value: "custom-x" } });
  expect(screen.getByTestId("val").textContent).toBe("custom-x");
});

// Escape undoes one step: the list first, the Settings drawer under it next.
test("Escape folds the list and leaves the drawer under it for the next one", () => {
  const closeDrawer = vi.fn();
  render(
    <>
      <OpenRailScreen id="settings" onClose={closeDrawer} />
      <Harness />
    </>,
  );
  const input = screen.getByTestId("cb");
  fireEvent.focus(input);
  expect(screen.getByRole("listbox")).toBeTruthy();
  fireEvent.keyDown(input, { key: "Escape" });
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(closeDrawer).not.toHaveBeenCalled();
  fireEvent.keyDown(input, { key: "Escape" });
  expect(closeDrawer).toHaveBeenCalledTimes(1);
});
