import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import { PlanDocumentSection } from "./PlanDocumentSection";
import { isEditorEmbed } from "../embedShell";

// Keep the real embedShell exports; override only the embed predicate so tests
// can flip between browser (false, the jsdom default) and plugin (true).
vi.mock("../embedShell", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../embedShell")>()),
  isEditorEmbed: vi.fn(() => false),
}));

afterEach(() => {
  cleanup();
  vi.mocked(isEditorEmbed).mockReturnValue(false);
});

function renderPlan(
  overrides?: Partial<Parameters<typeof PlanDocumentSection>[0]>,
) {
  return render(
    <PlanDocumentSection
      sessionId="sess_test"
      slug="demo-plan"
      name="Demo plan"
      overview="Short overview for the card"
      content="---\nname: Demo\n---\n# Hello\n\nSteps"
      body={`# Hello

Steps`}
      path="/tmp/sess_test/plans/demo-plan.plan.md"
      expanded
      onExpandedChange={() => {}}
      onDiscard={() => {}}
      onRunPlan={() => {}}
      {...overrides}
    />,
  );
}

test("preview is default and title shows absolute path in tooltip", () => {
  renderPlan();
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Hello");
  const title = screen.getByText("Demo plan");
  expect(title).toHaveAttribute(
    "title",
    "/tmp/sess_test/plans/demo-plan.plan.md",
  );
  expect(
    screen.getByRole("button", { name: "Toggle preview" }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("eye toggle switches preview and markdown in one pane", () => {
  renderPlan();
  const pane = document.querySelector(".plan-document-pane");
  expect(pane).toBeTruthy();
  const toggle = screen.getByRole("button", { name: "Toggle preview" });
  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute("aria-pressed", "false");
  expect(screen.getByRole("textbox", { name: /plan body/i })).toBeInTheDocument();
  expect(screen.getByTestId("plan_editor_gutter")).toBeInTheDocument();
  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Hello");
});

test("discarded plan disables run and keeps card visible", () => {
  renderPlan({ discarded: true });
  expect(screen.getByRole("button", { name: /run plan/i })).toBeDisabled();
  expect(screen.getByRole("button", { name: /discard/i })).toBeDisabled();
});

test("collapsed card shows title and one-line description", () => {
  renderPlan({ expanded: false });
  expect(screen.getByText("Demo plan")).toBeInTheDocument();
  expect(screen.getByText("Short overview for the card")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Toggle preview" })).toBeNull();
});

test("markdown edit autosaves body with transcript content for bootstrap", async () => {
  vi.useFakeTimers();
  const fetchMock = vi.fn().mockResolvedValue({ ok: true });
  vi.stubGlobal("fetch", fetchMock);
  try {
    renderPlan();
    fireEvent.click(screen.getByRole("button", { name: "Toggle preview" }));
    const editor = screen.getByRole("textbox", { name: /plan body/i });
    fireEvent.change(editor, { target: { value: "# Hello\n\nEdited" } });
    await vi.advanceTimersByTimeAsync(650);
    expect(fetchMock).toHaveBeenCalled();
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/foxxycode/sessions/sess_test/plans/demo-plan");
    expect(init?.method).toBe("PUT");
    const payload = JSON.parse(String(init?.body));
    expect(payload.body).toBe("# Hello\n\nEdited");
    expect(payload.content).toContain("name: Demo");
  } finally {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  }
});

test("a transcript merge does not clobber an unsaved markdown draft", async () => {
  vi.useFakeTimers();
  const fetchMock = vi.fn().mockResolvedValue({ ok: true });
  vi.stubGlobal("fetch", fetchMock);
  try {
    const { rerender } = renderPlan();
    fireEvent.click(screen.getByRole("button", { name: "Toggle preview" }));
    const editor = screen.getByRole("textbox", { name: /plan body/i });
    fireEvent.change(editor, { target: { value: "# Hello\n\nTyping in flight" } });

    // A loadMessages merge re-renders the card with a body written from another
    // window while the debounce is still pending; the local draft must win.
    rerender(
      <PlanDocumentSection
        sessionId="sess_test"
        slug="demo-plan"
        name="Demo plan"
        overview="Short overview for the card"
        content="---\nname: Demo\n---\n# Hello\n\nFrom the other window"
        body={"# Hello\n\nFrom the other window"}
        path="/tmp/sess_test/plans/demo-plan.plan.md"
        expanded
        onExpandedChange={() => {}}
        onDiscard={() => {}}
        onRunPlan={() => {}}
      />,
    );
    expect(
      screen.getByRole("textbox", { name: /plan body/i }),
    ).toHaveValue("# Hello\n\nTyping in flight");

    // Once the save lands, the server echoing our own text back is accepted.
    await vi.advanceTimersByTimeAsync(650);
    rerender(
      <PlanDocumentSection
        sessionId="sess_test"
        slug="demo-plan"
        name="Demo plan"
        overview="Short overview for the card"
        content="---\nname: Demo\n---\n# Hello\n\nTyping in flight"
        body={"# Hello\n\nTyping in flight"}
        path="/tmp/sess_test/plans/demo-plan.plan.md"
        expanded
        onExpandedChange={() => {}}
        onDiscard={() => {}}
        onRunPlan={() => {}}
      />,
    );
    expect(
      screen.getByRole("textbox", { name: /plan body/i }),
    ).toHaveValue("# Hello\n\nTyping in flight");
  } finally {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  }
});

test("View in IDE is hidden in the browser shell", () => {
  renderPlan();
  expect(screen.queryByTestId("plan_document_open_in_ide")).toBeNull();
});

test("View in IDE is an icon button in the pane tool rail, before the eye", async () => {
  vi.mocked(isEditorEmbed).mockReturnValue(true);
  const fetchMock = vi.fn().mockResolvedValue({ ok: true });
  vi.stubGlobal("fetch", fetchMock);
  try {
    renderPlan();

    const openBtn = screen.getByTestId("plan_document_open_in_ide");
    const eyeBtn = screen.getByRole("button", { name: "Toggle preview" });
    // Both live in the floating rail over the body, not in the footer.
    const rail = document.querySelector(".plan-document-pane-tools");
    expect(rail).toBeTruthy();
    expect(rail!.contains(openBtn)).toBe(true);
    expect(rail!.contains(eyeBtn)).toBe(true);
    expect(document.querySelector(".plan-document-foot")!.contains(openBtn)).toBe(
      false,
    );
    // Node.compareDocumentPosition: FOLLOWING (4) means the eye comes after it.
    expect(openBtn.compareDocumentPosition(eyeBtn)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );

    // Icon-only: the label rides on title/aria-label so the button stays compact.
    expect(openBtn).toHaveAttribute("aria-label", "View in IDE");
    expect(openBtn).toHaveAttribute("title", "View in IDE");
    expect(openBtn.textContent?.trim()).toBe("");
    expect(openBtn.querySelector("svg.plan-document-ide-svg")).toBeTruthy();

    fireEvent.click(openBtn);
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(
      "/foxxycode/sessions/sess_test/plans/demo-plan/open-in-ide",
    );
    expect(init?.method).toBe("POST");
  } finally {
    vi.unstubAllGlobals();
  }
});

test("a failed View in IDE surfaces an error on the card", async () => {
  vi.mocked(isEditorEmbed).mockReturnValue(true);
  const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 404 });
  vi.stubGlobal("fetch", fetchMock);
  try {
    renderPlan();
    fireEvent.click(screen.getByTestId("plan_document_open_in_ide"));
    await screen.findByText("Could not open the plan in the IDE");
  } finally {
    vi.unstubAllGlobals();
  }
});

test("View in IDE is disabled for a discarded plan", () => {
  vi.mocked(isEditorEmbed).mockReturnValue(true);
  renderPlan({ discarded: true });
  expect(screen.getByTestId("plan_document_open_in_ide")).toBeDisabled();
});

// The tool rail lives in the expanded body, so a collapsed card has no icons.
test("View in IDE is not rendered while the card is collapsed", () => {
  vi.mocked(isEditorEmbed).mockReturnValue(true);
  renderPlan({ expanded: false });
  expect(screen.queryByTestId("plan_document_open_in_ide")).toBeNull();
});

test("a plan rewritten by the model replaces an untouched body", () => {
  const { rerender } = renderPlan();
  rerender(
    <PlanDocumentSection
      sessionId="sess_test"
      slug="demo-plan"
      name="Demo plan"
      overview="Short overview for the card"
      content="---\nname: Demo\n---\n# Hello\n\nRewritten by plan_write"
      body={"# Hello\n\nRewritten by plan_write"}
      path="/tmp/sess_test/plans/demo-plan.plan.md"
      expanded
      onExpandedChange={() => {}}
      onDiscard={() => {}}
      onRunPlan={() => {}}
    />,
  );
  expect(
    document.querySelector(".plan-document-preview-pane")?.textContent,
  ).toContain("Rewritten by plan_write");
});
