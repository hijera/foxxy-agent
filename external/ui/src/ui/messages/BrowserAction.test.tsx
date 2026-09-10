import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { ToolCallMessage } from "./ToolCallMessage";
import { setLocale } from "../i18n/i18n";

afterEach(() => {
  cleanup();
  setLocale("en");
});

test("a tool error does not claim that a screenshot was taken", () => {
  render(
    <ToolCallMessage
      toolCallId="shot-error"
      title="foxxycode_browser_screenshot"
      status="completed"
      resultText="error: capture failed"
    />,
  );
  expect(screen.queryByText("Screenshot taken")).toBeNull();
  expect(screen.getByText("Take screenshot")).toBeTruthy();
  expect(screen.getByText("error: capture failed")).toBeTruthy();
});

test("zero displacement has no arrow", () => {
  const { container } = render(
    <ToolCallMessage
      toolCallId="zero"
      title="foxxycode_browser_scroll"
      status="completed"
      argsText="{}"
    />,
  );
  expect(screen.getByText("No displacement")).toBeTruthy();
  expect(container.querySelector(".browser-scroll-arrow")).toBeNull();
});

test("uses localized screenshot completion and handles unavailable assets", () => {
  setLocale("ru");
  const { container } = render(
    <ToolCallMessage
      toolCallId="shot"
      title="foxxycode_browser_screenshot"
      status="completed"
      resultText={
        "captured screenshot\nscreenshot: C:\\sessions\\s1\\assets\\browser_1.png"
      }
      sessionId="s1"
    />,
  );
  expect(screen.getByText("Снят скриншот")).toBeTruthy();
  const img = container.querySelector("img")!;
  expect(img.getAttribute("src")).toBe(
    "/foxxycode/sessions/s1/assets/browser_1.png",
  );
  fireEvent.error(img);
  expect(screen.getByText("Скриншот недоступен")).toBeTruthy();
});

test("loads the full browser result instead of re-rendering the truncated preview", async () => {
  const fetch = vi.fn().mockResolvedValue(undefined);
  const { container, rerender } = render(
    <ToolCallMessage
      toolCallId="read"
      title="foxxycode_browser_read_page"
      status="completed"
      resultText="page\n..."
      resultWasTruncated
      onFetchToolCallFull={fetch}
    />,
  );
  fireEvent.click(screen.getByTestId("tool-result-more"));
  await waitFor(() => expect(fetch).toHaveBeenCalledWith("read"));
  rerender(
    <ToolCallMessage
      toolCallId="read"
      title="foxxycode_browser_read_page"
      status="completed"
      resultText="page\n..."
      fullResultText={"page\n  heading Example\n  button Submit"}
      resultWasTruncated
      onFetchToolCallFull={fetch}
    />,
  );
  await waitFor(() => expect(container.textContent).toContain("button Submit"));
});

test("recovers truncated browser arguments after restoring history", async () => {
  const fetch = vi.fn().mockResolvedValue(undefined);
  render(
    <ToolCallMessage
      toolCallId="args"
      title="foxxycode_browser_evaluate"
      status="completed"
      argsText={'{"expression":"document.'}
      onFetchToolCallFull={fetch}
    />,
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledWith("args"));
});

test("treats markup in selectors and results as text", () => {
  const { container } = render(
    <ToolCallMessage
      toolCallId="escape"
      title="foxxycode_browser_click"
      status="failed"
      argsText={JSON.stringify({ selector: '<img src=x onerror="alert(1)">' })}
      resultText={"error: <script>alert(1)</script>"}
    />,
  );
  expect(container.querySelector("img,script")).toBeNull();
  expect(container.textContent).toContain("<script>alert(1)</script>");
});

test("shows the browser action and arguments before execution", () => {
  render(
    <ToolCallMessage
      toolCallId="nav"
      title="foxxycode_browser_navigate"
      status="in_progress"
      argsText={'{"url":"https://example.com"}'}
    />,
  );
  expect(screen.getByText("Open page")).toBeTruthy();
  expect(screen.getByText("https://example.com")).toBeTruthy();
});

test("formats and highlights JavaScript and preserves the complete result", async () => {
  const { container } = render(
    <ToolCallMessage
      toolCallId="eval"
      title="foxxycode_browser_evaluate"
      status="completed"
      argsText={JSON.stringify({
        expression: "(()=>{const count=2;return {count};})()",
      })}
      resultText={'result: {"count":2}'}
    />,
  );
  expect(screen.getByText("Execute code")).toBeTruthy();
  await waitFor(() =>
    expect(
      container.querySelector("code.language-javascript")?.textContent,
    ).toContain("const count = 2;"),
  );
  expect(container.querySelector(".hljs-keyword")).toBeTruthy();
  expect(container.textContent).toContain('"count": 2');
});

test("shows displacement inside a screen and gives selectors precedence", () => {
  const { container, rerender } = render(
    <ToolCallMessage
      toolCallId="scroll"
      title="foxxycode_browser_scroll"
      status="completed"
      argsText={'{"x":-120,"y":400}'}
    />,
  );
  expect(
    screen.getByRole("img", { name: "Scroll offset: X -120 px, Y +400 px" }),
  ).toBeTruthy();
  expect(container.querySelector(".browser-scroll-arrow")).toBeTruthy();
  rerender(
    <ToolCallMessage
      toolCallId="scroll"
      title="foxxycode_browser_scroll"
      status="completed"
      argsText={'{"selector":"#footer","x":-120,"y":400}'}
    />,
  );
  expect(container.querySelector(".browser-scroll-screen")).toBeNull();
  expect(screen.getByText("#footer")).toBeTruthy();
});

test("preserves multiline errors and never shows a disabled screenshot", () => {
  const { container } = render(
    <ToolCallMessage
      toolCallId="bad"
      title="foxxycode_browser_click"
      status="failed"
      argsText={'{"selector":"#go"}'}
      resultText={
        "error: click failed\nsecond diagnostic\nscreenshot: disabled"
      }
    />,
  );
  expect(container.textContent).toContain("second diagnostic");
  expect(container.querySelector("img")).toBeNull();
});
