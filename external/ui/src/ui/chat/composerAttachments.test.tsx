import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Composer } from "./Composer";
import { initLocale } from "../i18n/i18n";

afterEach(() => {
  cleanup();
  initLocale("en");
  vi.unstubAllGlobals();
});

function imageFile(name = "clip.png"): File {
  return new File([new Uint8Array([1, 2, 3])], name, { type: "image/png" });
}

/** A paste carries several flavours; only the file items are attachments. */
function pasteWithImage(el: Element, files: File[]) {
  fireEvent.paste(el, {
    clipboardData: {
      items: files.map((f) => ({
        kind: "file",
        type: f.type,
        getAsFile: () => f,
      })),
      files,
      getData: () => "",
    },
  });
}

function renderComposer(props: {
  multimodal?: boolean;
  onSend?: (text: string, files?: File[]) => void;
}) {
  return render(
    <Composer
      value=""
      isEmpty={false}
      mode="agent"
      modes={["agent", "plan"]}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={props.onSend ?? (() => {})}
      {...(props.multimodal === undefined
        ? {}
        : { llmModelMultimodal: props.multimodal })}
    />,
  );
}

test("pasting an image attaches it under a distinguishable name", () => {
  renderComposer({ multimodal: true });
  const ta = screen.getByLabelText("Message");

  pasteWithImage(ta, [imageFile()]);
  pasteWithImage(ta, [imageFile()]);

  // Browsers name every clipboard image the same, so a second paste must not be
  // indistinguishable from the first.
  const chips = screen.getAllByTestId("composer-attachment-chip");
  expect(chips).toHaveLength(2);
  expect(chips[0]!.textContent).toContain("pasted-1.png");
  expect(chips[1]!.textContent).toContain("pasted-2.png");
});

test("a paste for a model that cannot take attachments is refused with a notice", () => {
  renderComposer({ multimodal: false });
  const ta = screen.getByLabelText("Message");

  pasteWithImage(ta, [imageFile()]);

  expect(screen.queryAllByTestId("composer-attachment-chip")).toHaveLength(0);
  expect(screen.getByTestId("composer-attach-hint").textContent).toBe(
    "The selected model cannot accept attachments",
  );
});

test("an attachment alone is a valid message", () => {
  const sent: { text: string; files?: File[] }[] = [];
  renderComposer({
    multimodal: true,
    onSend: (text, files) => sent.push({ text, ...(files ? { files } : {}) }),
  });
  const ta = screen.getByLabelText("Message");

  const send = screen.getByLabelText("Send") as HTMLButtonElement;
  expect(send.disabled).toBe(true);

  pasteWithImage(ta, [imageFile()]);
  expect((screen.getByLabelText("Send") as HTMLButtonElement).disabled).toBe(
    false,
  );

  fireEvent.click(screen.getByLabelText("Send"));
  expect(sent).toHaveLength(1);
  expect(sent[0]!.text).toBe("");
  expect(sent[0]!.files).toHaveLength(1);
});

test("attachments held by the parent render as disabled chips for a plain model", () => {
  render(
    <Composer
      value="hi"
      isEmpty={false}
      mode="agent"
      modes={["agent", "plan"]}
      attachedFiles={[imageFile("held.png")]}
      onAttachedFilesChange={() => {}}
      llmModelMultimodal={false}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
    />,
  );

  const chip = screen.getByTestId("composer-attachment-chip");
  expect(chip.className).toContain("composer-attachment-chip--disabled");
  expect(chip.getAttribute("aria-disabled")).toBe("true");
});
