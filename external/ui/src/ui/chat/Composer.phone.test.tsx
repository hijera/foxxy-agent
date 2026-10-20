import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Composer } from "./Composer";
import { DEFAULT_SEND_MODE, setSendMode } from "../i18n/sendModeConfig";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); setSendMode(DEFAULT_SEND_MODE); });
function mount(touchOnly: boolean, narrow: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query.includes("pointer") ? touchOnly : narrow, media: query,
    addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {},
    onchange: null, dispatchEvent: () => false,
  }));
  const onSend = vi.fn(); const onChange = vi.fn();
  render(<Composer value="hello" isEmpty={false} mode="agent" modes={["agent"]}
    onModeChange={() => {}} onSend={onSend} onChange={onChange} />);
  return { onSend, onChange, ta: screen.getByRole("textbox", {name:"Message"}) as HTMLTextAreaElement };
}
test("a narrow keyboard window obeys send_mode", () => {
  const {onSend, ta} = mount(false, true);
  fireEvent.keyDown(ta, {key:"Enter"}); expect(onSend).toHaveBeenCalledWith("hello");
});
test("a wide touch-only device keeps Return as a newline", () => {
  const {onSend, ta} = mount(true, false);
  fireEvent.keyDown(ta, {key:"Enter"}); expect(onSend).not.toHaveBeenCalled();
});
test("Ctrl+Enter inserts a newline at the selection in enter mode", () => {
  const {onSend, onChange, ta} = mount(false, true); ta.setSelectionRange(1,4);
  fireEvent.keyDown(ta, {key:"Enter", ctrlKey:true});
  expect(onSend).not.toHaveBeenCalled(); expect(onChange).toHaveBeenCalledWith("h\no");
});
test("IME confirmation does not send", () => {
  const {onSend, ta} = mount(false, false);
  fireEvent.keyDown(ta, {key:"Enter", isComposing:true}); expect(onSend).not.toHaveBeenCalled();
});
test("the workspace strip keeps enhance outside its scrolling box", () => {
  mount(true,true);
  expect(document.querySelector(".composer-context-scroll")).not.toBeNull();
  expect(document.querySelector(".composer-context-scroll .composer-enhance-btn")).toBeNull();
});

test("Shift+Enter leaves the newline to the browser and does not send", () => {
  const { onSend, ta } = mount(false, true);
  expect(fireEvent.keyDown(ta, { key: "Enter", shiftKey: true })).toBe(true);
  expect(onSend).not.toHaveBeenCalled();
});

test("a touch-only phone: Return inserts a newline and the Send button sends", () => {
  const { onSend, ta } = mount(true, true);
  expect(fireEvent.keyDown(ta, { key: "Enter" })).toBe(true);
  expect(onSend).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(onSend).toHaveBeenCalledWith("hello");
});

test("the keyboard's Enter key is labelled send, or enter on a touch-only phone", () => {
  let view = mount(false, true);
  expect(view.ta.getAttribute("enterkeyhint")).toBe("send");
  cleanup();
  view = mount(true, true);
  expect(view.ta.getAttribute("enterkeyhint")).toBe("enter");
});
