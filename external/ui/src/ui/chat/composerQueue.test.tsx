import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Composer } from "./Composer";

afterEach(() => cleanup());

// While a turn runs, the composer is not a dead end: a draft with text in it is
// a follow-up for the queue, an empty one still leaves the Stop control.
function renderGenerating(opts: {
  value: string;
  queued?: { id: string; text: string }[];
  onQueue?: (text: string, mode: "steer" | "after_turn", files?: File[]) => void;
  onStop?: () => void;
  onCancelQueued?: (id: string) => void;
}) {
  const el = (value: string) => (
    <Composer
      value={value}
      isEmpty={false}
      mode="agent"
      modes={["agent", "plan"]}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
      generating={true}
      onStop={opts.onStop ?? (() => {})}
      queuedMessages={opts.queued ?? []}
      onQueue={opts.onQueue ?? (() => {})}
      queueMode="steer"
      onCancelQueued={opts.onCancelQueued ?? (() => {})}
    />
  );
  const view = render(el(opts.value));
  return { ...view, setValue: (value: string) => view.rerender(el(value)) };
}

test("the primary control queues the draft while a turn runs", () => {
  const onQueue = vi.fn();
  const onStop = vi.fn();
  renderGenerating({ value: "check the Windows path too", onQueue, onStop });

  const btn = screen.getByRole("button", { name: "Queue this message" });
  expect(btn).toHaveAttribute("data-queue", "true");
  fireEvent.click(btn);

  expect(onQueue).toHaveBeenCalledWith("check the Windows path too", "steer", []);
  expect(onStop).not.toHaveBeenCalled();
});

test("an empty draft leaves the control as Stop", () => {
  const onQueue = vi.fn();
  const onStop = vi.fn();
  renderGenerating({ value: "   ", onQueue, onStop });

  const btn = screen.getByRole("button", { name: "Stop generation" });
  expect(btn).not.toHaveAttribute("data-queue");
  fireEvent.click(btn);

  expect(onStop).toHaveBeenCalledTimes(1);
  expect(onQueue).not.toHaveBeenCalled();
});

test("Enter queues the draft instead of being swallowed", () => {
  const onQueue = vi.fn();
  renderGenerating({ value: "one more thing", onQueue });

  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });

  expect(onQueue).toHaveBeenCalledWith("one more thing", "steer", []);
});

test("first queued message asks for the Enter preference once", () => {
  const onQueue = vi.fn();
  const onQueueModeChange = vi.fn();
  render(<Composer value="check this" isEmpty={false} mode="agent" modes={["agent"]} generating={true} onModeChange={() => {}} onChange={() => {}} onSend={() => {}} onQueue={onQueue} onQueueModeChange={onQueueModeChange} />);
  fireEvent.keyDown(screen.getByRole("textbox", { name: "Message" }), { key: "Enter" });
  expect(onQueue).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "After this turn" }));
  expect(onQueueModeChange).toHaveBeenCalledWith("after_turn");
  expect(onQueue).toHaveBeenCalledWith("check this", "after_turn", []);
});

test("queued messages are listed in order, each with its own remove control", () => {
  const onCancelQueued = vi.fn();
  renderGenerating({
    value: "",
    queued: [
      { id: "q_1", text: "first correction" },
      { id: "q_2", text: "second correction" },
    ],
    onCancelQueued,
  });

  const rows = screen.getAllByTestId("composer-queue-item");
  expect(rows).toHaveLength(2);
  expect(rows[0]).toHaveTextContent("first correction");
  expect(rows[1]).toHaveTextContent("second correction");

  fireEvent.click(screen.getByTestId("composer-queue-remove-q_2"));
  expect(onCancelQueued).toHaveBeenCalledWith("q_2");
});

test("nothing queued renders no list at all", () => {
  renderGenerating({ value: "", queued: [] });
  expect(screen.queryByTestId("composer-queue")).toBeNull();
});

test("the placeholder says a draft joins the running turn", () => {
  renderGenerating({ value: "" });
  expect(screen.getByRole("textbox")).toHaveAttribute(
    "placeholder",
    "Add a follow-up for the running turn",
  );
});

test("a first message sent with Tab still goes the other way once Enter's mode is chosen", () => {
  const onQueue = vi.fn();
  const onQueueModeChange = vi.fn();
  render(<Composer value="review the answer" isEmpty={false} mode="agent" modes={["agent"]} generating={true} onModeChange={() => {}} onChange={() => {}} onSend={() => {}} onQueue={onQueue} onQueueModeChange={onQueueModeChange} />);
  fireEvent.keyDown(screen.getByRole("textbox", { name: "Message" }), { key: "Tab" });
  expect(onQueue).not.toHaveBeenCalled();
  expect(screen.getByTestId("composer-queue-choice")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Steer now" }));
  expect(onQueueModeChange).toHaveBeenCalledWith("steer");
  expect(onQueue).toHaveBeenCalledWith("review the answer", "after_turn", []);
  expect(screen.queryByTestId("composer-queue-choice")).not.toBeInTheDocument();
});

test("a queued message with images shows a paperclip and their count, not the images", () => {
  render(
    <Composer
      value=""
      isEmpty={true}
      mode="agent"
      modes={["agent"]}
      generating={true}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
      onQueue={() => {}}
      queueMode="steer"
      queuedMessages={[{ id: "q1", text: "compare", mode: "after_turn", imageParts: [{ name: "a.png", mimeType: "image/png", sizeBytes: 3 }, { name: "b.png" }] }]}
    />,
  );
  const files = screen.getByTestId("composer-queue-files-q1");
  expect(files).toHaveTextContent("2");
  expect(files).toHaveAttribute("aria-label", "2 images attached");
  expect(files.querySelector("svg")).not.toBeNull();
  expect(screen.getByTestId("composer-queue-mode-q1")).toHaveTextContent("After turn");
});

// The draft is cleared only on the next render, so a second activation of a
// queue action in that window still reads the text of the one it belongs to.
// Both would POST it to /queue, and the operator would see two identical rows.
test("a second activation of a queue action takes no second copy of the draft", () => {
  const onQueue = vi.fn();
  renderGenerating({ value: "check the Windows path too", onQueue });
  const box = screen.getByRole("textbox");
  fireEvent.keyDown(box, { key: "Tab" });
  fireEvent.keyDown(box, { key: "Tab" });
  fireEvent.click(screen.getByRole("button", { name: "Queue this message" }));

  expect(onQueue).toHaveBeenCalledTimes(1);
  expect(onQueue).toHaveBeenCalledWith("check the Windows path too", "after_turn", []);
});

// An attachment-only queue empties its own chips only on the next render, so a
// second activation in that window sees the same files and would queue them
// again - with no text for the draft guard to match on. Both clicks land before
// the composer re-renders, the way a double click on a controlled chip row
// does.
test("a second activation of a queue action takes no second copy of the attachments", async () => {
  const onQueue = vi.fn();
  render(
    <Composer
      value=""
      isEmpty={true}
      mode="agent"
      modes={["agent"]}
      generating={true}
      llmModelMultimodal={true}
      queueMode="steer"
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
      onQueue={onQueue}
    />,
  );
  const file = new File(["image"], "img.png", { type: "image/png" });
  fireEvent.change(screen.getByTestId("composer-file-input"), { target: { files: [file] } });
  await waitFor(() => screen.getByText("img.png"));
  const queueButton = screen.getByRole("button", { name: "Queue this message" });
  await act(async () => {
    queueButton.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    queueButton.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });

  expect(onQueue).toHaveBeenCalledTimes(1);
  expect(onQueue).toHaveBeenCalledWith("", "steer", [file]);
});

// Enter's rule (composerEnterAction) ignores a held key's repeats; Tab is the
// same kind of send and must not queue once per repeat either.
test("a held Tab queues on its first press only, not on the repeats", () => {
  const onQueue = vi.fn();
  renderGenerating({ value: "one more thing", onQueue });
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Tab", repeat: true });

  expect(onQueue).not.toHaveBeenCalled();
});

// The guard is one queue per draft, not one per composer: what the app cleared
// and the operator typed over is a follow-up of its own.
test("a draft that changed is a new message of its own", () => {
  const onQueue = vi.fn();
  const { setValue } = renderGenerating({ value: "first follow-up", onQueue });
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Tab" });
  setValue("second follow-up");
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Tab" });

  expect(onQueue).toHaveBeenCalledTimes(2);
  expect(onQueue).toHaveBeenNthCalledWith(1, "first follow-up", "after_turn", []);
  expect(onQueue).toHaveBeenNthCalledWith(2, "second follow-up", "after_turn", []);
});
