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
import { PermissionToolPreview } from "../chat/PermissionPromptPreview";
import { buildToolCallPreview } from "../chat/permissionToolPreview";

afterEach(() => {
  cleanup();
  setLocale("en");
});

test("restored ACP approval arguments use the structured SVN preview", () => {
  const argsText =
    'Arguments: {"source":"branches/release","revision":"12:14"}';
  render(
    <PermissionToolPreview
      preview={buildToolCallPreview({ title: "svn_merge", argsText }, argsText)}
    />,
  );
  expect(screen.getByText("branches/release")).toBeTruthy();
  expect(screen.getByText("12:14")).toBeTruthy();
});

test("shows SVN paths and distinguishes text, property and tree conflicts", () => {
  const { container } = render(
    <ToolCallMessage
      toolCallId="status"
      title="svn_status"
      status="completed"
      argsText="{}"
      resultText={
        "M       src/main.go\n C      props.txt\n      C tree.txt\n?       new file.txt"
      }
    />,
  );
  expect(screen.getByText("SVN · Working copy status")).toBeTruthy();
  expect(screen.getByText("src/main.go")).toBeTruthy();
  expect(screen.getByText("new file.txt")).toBeTruthy();
  expect(container.querySelectorAll(".svn-status-row--conflict")).toHaveLength(
    2,
  );
});

test("a completed transport with an SVN error is visibly failed", () => {
  render(
    <ToolCallMessage
      toolCallId="commit"
      title="svn_commit"
      status="completed"
      argsText={'{"message":"Fix encoding","paths":["src/main.go"]}'}
      resultText="error: svn: E155015: unresolved conflicts"
    />,
  );
  expect(screen.getAllByText("Operation failed")).toHaveLength(2);
  expect(screen.getByText("Fix encoding")).toBeTruthy();
  expect(screen.queryByText("Completed")).toBeNull();
});

test("highlights a diff without dropping property changes or binary diagnostics", () => {
  const result =
    "Index: src/main.go\n--- src/main.go\n+++ src/main.go\n@@ -1 +1 @@\n-old\n+new\nProperty changes on: src/main.go\nCannot display: file marked as a binary type.";
  const { container } = render(
    <ToolCallMessage
      toolCallId="diff"
      title="svn_diff"
      status="completed"
      resultText={result}
    />,
  );
  expect(container.querySelector(".svn-diff-line--add")?.textContent).toBe(
    "+new\n",
  );
  expect(container.querySelector(".svn-diff-line--del")?.textContent).toBe(
    "-old\n",
  );
  expect(container.querySelector(".svn-diff")?.textContent).toBe(result);
});

test("permission preview shows the commit message, scope and pending state", () => {
  const argsText = '{"message":"Fix encoding","paths":[]}';
  render(
    <PermissionToolPreview
      preview={buildToolCallPreview(
        { title: "svn_commit", argsText },
        argsText,
      )}
    />,
  );
  expect(screen.getByText("Fix encoding")).toBeTruthy();
  expect(screen.getByText("Entire working copy")).toBeTruthy();
  expect(screen.queryByText("Completed")).toBeNull();
});

test("preserves malformed arguments and cancelled output, with no false success", () => {
  render(
    <ToolCallMessage
      toolCallId="cancel"
      title="svn_update"
      status="cancelled"
      argsText={'{"paths":['}
      resultText="U    src/main.go"
    />,
  );
  expect(screen.getByText('{"paths":[')).toBeTruthy();
  expect(screen.getByText("Cancelled")).toBeTruthy();
  expect(screen.queryByText("Completed")).toBeNull();
});

test("loads full SVN arguments and results restored from history", async () => {
  const fetch = vi.fn(async () => {});
  const { rerender } = render(
    <ToolCallMessage
      toolCallId="history"
      title="svn_log"
      status="completed"
      argsText={'{"target":'}
      resultText="preview"
      resultWasTruncated
      onFetchToolCallFull={fetch}
    />,
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledWith("history"));
  fireEvent.click(screen.getByTestId("tool-result-more"));
  rerender(
    <ToolCallMessage
      toolCallId="history"
      title="svn_log"
      status="completed"
      argsText='{"target":"trunk"}'
      resultText="preview"
      fullResultText="r12 | alice | 2026-09-10 | 1 line\n\nFull commit message"
      resultWasTruncated
      onFetchToolCallFull={fetch}
    />,
  );
  await waitFor(() =>
    expect(screen.getByText(/Full commit message/)).toBeTruthy(),
  );
});
