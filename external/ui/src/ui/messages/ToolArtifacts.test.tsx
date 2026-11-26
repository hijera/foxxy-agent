import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

import { setEnv } from "../env/remoteEnv";
import { parseToolArtifacts } from "../chat/toolArtifacts";
import { ToolCallMessage } from "./ToolCallMessage";
import { ArtifactCard, placeArtifactMenu } from "./ToolArtifactCards";

const artifact = {
  id: "artifact-1",
  name: "release-notes.pdf",
  sha256: "a".repeat(64),
  size: 2048,
  url: "/foxxycode/sessions/s1/artifacts/artifact-1",
};
const unavailableArtifact = (({ url: _url, ...rest }) => rest)(artifact);

afterEach(() => {
  cleanup();
  setEnv({ mode: "local" });
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  delete (document as unknown as { execCommand?: unknown }).execCommand;
});

test("normalizes artifact metadata and retains a missing download URL as unavailable", () => {
  expect(
    parseToolArtifacts([artifact, { ...artifact, id: "gone", url: "" }]),
  ).toEqual([artifact, { ...artifact, id: "gone", url: undefined }]);
  expect(parseToolArtifacts([{ ...artifact, sha256: "not-a-digest" }])).toEqual(
    [],
  );
});
test("drops artifact URLs that are not root-relative server routes", () => {
  const hostile = {
    ...artifact,
    url: "javascript:alert(1)",
    previewUrl: "https://evil.example/pixel",
    revealUrl: "//evil.example/reveal",
  };
  expect(parseToolArtifacts([hostile])).toEqual([
    { ...unavailableArtifact, previewUrl: undefined, revealUrl: undefined },
  ]);
  expect(parseToolArtifacts([{ ...artifact, url: "/foxxycode/ok" }])).toEqual([
    { ...artifact, url: "/foxxycode/ok" },
  ]);
});

test("renders completed share_file artifacts below the closed disclosure", () => {
  render(
    <ToolCallMessage
      toolCallId="share-1"
      title="share_file"
      status="completed"
      artifacts={[artifact]}
    />,
  );

  expect(screen.getByTestId("tool-artifact-card-artifact-1")).toBeVisible();
  expect(screen.getByText("release-notes.pdf")).toBeVisible();
  expect(screen.getByText("PDF · 2.0 KB")).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "Download release-notes.pdf" }),
  ).toBeNull();
  expect(
    screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
  ).toBeVisible();
  expect(screen.getByTestId("tool-details-share-1")).not.toHaveAttribute(
    "open",
  );
});

test("does not render artifacts for a pending or unrelated tool call", () => {
  const { rerender } = render(
    <ToolCallMessage
      toolCallId="share-2"
      title="share_file"
      status="in_progress"
      artifacts={[artifact]}
    />,
  );
  expect(screen.queryByTestId("tool-artifact-card-artifact-1")).toBeNull();

  rerender(
    <ToolCallMessage
      toolCallId="other-1"
      title="write"
      status="completed"
      artifacts={[artifact]}
    />,
  );
  expect(screen.queryByTestId("tool-artifact-card-artifact-1")).toBeNull();
});

test("marks an artifact without a URL as unavailable", () => {
  render(
    <ToolCallMessage
      toolCallId="share-3"
      title="share_file"
      status="completed"
      artifacts={[unavailableArtifact]}
    />,
  );

  expect(screen.getByText("Download unavailable")).toBeVisible();
  fireEvent.click(
    screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
  );
  expect(screen.getByRole("menuitem", { name: "Download" })).toBeDisabled();
});

test("opens an inline image artifact in the shared lightbox", () => {
  const { container } = render(
    <ArtifactCard
      inline
      artifact={{
        ...artifact,
        name: "release-overview.png",
        previewUrl: "/foxxycode/sessions/s1/artifacts/artifact-1/preview",
      }}
    />,
  );

  fireEvent.click(
    screen.getByRole("button", { name: "Open release-overview.png" }),
  );
  expect(screen.getByRole("dialog")).toHaveTextContent("release-overview.png");
  expect(
    screen.getByRole("img", { name: "release-overview.png" }),
  ).toHaveAttribute(
    "src",
    "/foxxycode/sessions/s1/artifacts/artifact-1/preview",
  );
  expect(
    container.querySelector(".inline-artifact-extension")?.parentElement,
  ).toHaveClass("tool-artifact-card");
});

test("opens the actions menu outside the card, which clips what overflows it", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  const card = screen.getByTestId("inline-artifact-card-artifact-1");
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });

  fireEvent.click(trigger);
  const menu = screen.getByRole("menu");
  expect(card).not.toContainElement(menu);
  expect(menu.parentElement).toBe(document.body);
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  expect(trigger).toHaveAttribute("aria-haspopup", "menu");
  expect(screen.getAllByRole("menuitem")).toHaveLength(6);
  // The first item that can be picked takes the focus, so the keyboard starts
  // inside the menu (Mention source is disabled: this file names no source).
  expect(
    screen.getByRole("menuitem", { name: "Mention source" }),
  ).toBeDisabled();
  expect(screen.getByRole("menuitem", { name: "Copy name" })).toHaveFocus();
});

test("draws the trigger's dots as an svg, not a text glyph", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  expect(trigger.querySelector("svg")).not.toBeNull();
  expect(trigger.textContent).toBe("");
});

test("the trigger closes the menu it opened", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });

  fireEvent.click(trigger);
  expect(screen.getByRole("menu")).toBeVisible();
  // A real click starts with a press on the trigger, which is outside the menu.
  fireEvent.mouseDown(trigger);
  fireEvent.click(trigger);
  expect(screen.queryByRole("menu")).toBeNull();
  expect(trigger).toHaveAttribute("aria-expanded", "false");
});

test("a press outside the card and the menu closes the menu", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  fireEvent.click(
    screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
  );
  fireEvent.mouseDown(document.body);
  expect(screen.queryByRole("menu")).toBeNull();
});

test("Escape closes the menu wherever the focus is and hands it back to the trigger", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  fireEvent.click(trigger);
  trigger.focus();

  fireEvent.keyDown(document.activeElement!, { key: "Escape" });
  expect(screen.queryByRole("menu")).toBeNull();
  expect(trigger).toHaveFocus();
});

test("the arrow keys move between the items that can be picked", () => {
  render(
    <ArtifactCard
      inline
      onMention={() => {}}
      artifact={{
        ...unavailableArtifact,
        sourcePath: "/work/release-notes.pdf",
      }}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
  );
  const menu = screen.getByRole("menu");
  expect(
    screen.getByRole("menuitem", { name: "Mention source" }),
  ).toHaveFocus();

  fireEvent.keyDown(menu, { key: "ArrowDown" });
  expect(screen.getByRole("menuitem", { name: "Copy name" })).toHaveFocus();
  // Download is disabled without a URL and Reveal without a reveal URL, so up
  // from the first item wraps to Copy absolute path.
  fireEvent.keyDown(menu, { key: "ArrowUp" });
  fireEvent.keyDown(menu, { key: "ArrowUp" });
  expect(screen.getByRole("menuitem", { name: "Download" })).toBeDisabled();
  expect(
    screen.getByRole("menuitem", { name: "Copy absolute path" }),
  ).toHaveFocus();
  fireEvent.keyDown(menu, { key: "Home" });
  expect(
    screen.getByRole("menuitem", { name: "Mention source" }),
  ).toHaveFocus();
});

test("the menu follows its trigger while the page scrolls and closes once the trigger has left the window", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  const at = (top: number) =>
    vi
      .spyOn(trigger, "getBoundingClientRect")
      .mockReturnValue(
        DOMRect.fromRect({ x: 272, y: top, width: 28, height: 28 }),
      );

  at(100);
  fireEvent.click(trigger);
  expect(screen.getByRole("menu")).toHaveStyle({ top: "134px" });

  at(400);
  fireEvent.scroll(window);
  expect(screen.getByRole("menu")).toHaveStyle({ top: "434px" });

  at(-60);
  fireEvent.scroll(window);
  expect(screen.queryByRole("menu")).toBeNull();
});

// The transcript scrolls under the chat's sticky title and the docked composer
// inside the window, so a trigger can be hidden without leaving it; the menu
// used to stay open over the composer, following a trigger nobody could see.
test("closes the menu once something else covers its trigger", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue(
    DOMRect.fromRect({ x: 272, y: 300, width: 28, height: 28 }),
  );
  const composer = document.createElement("div");
  document.body.append(composer);
  let onTop: Element = trigger;
  const hit = vi.fn(() => onTop);
  Object.defineProperty(document, "elementFromPoint", {
    value: hit,
    configurable: true,
  });
  try {
    fireEvent.click(trigger);
    // The menu itself over the trigger (a window too short to keep them
    // apart) still counts as the trigger being there.
    onTop = screen.getByRole("menuitem", { name: "Copy name" });
    fireEvent.scroll(window);
    expect(screen.getByRole("menu")).toBeVisible();

    onTop = composer;
    fireEvent.scroll(window);
    expect(screen.queryByRole("menu")).toBeNull();
    expect(hit).toHaveBeenCalledWith(286, 314);
  } finally {
    delete (document as { elementFromPoint?: unknown }).elementFromPoint;
    composer.remove();
  }
});

// The focused item goes with the menu; without a hand-back the focus fell to
// the body and the next Tab started over from the top of the page.
test("a menu closed by its trigger scrolling out of sight hands the focus back to the trigger", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  const rect = vi
    .spyOn(trigger, "getBoundingClientRect")
    .mockReturnValue(
      DOMRect.fromRect({ x: 272, y: 300, width: 28, height: 28 }),
    );
  fireEvent.click(trigger);
  expect(screen.getByRole("menuitem", { name: "Copy name" })).toHaveFocus();

  rect.mockReturnValue(
    DOMRect.fromRect({ x: 272, y: -80, width: 28, height: 28 }),
  );
  fireEvent.scroll(window);
  expect(screen.queryByRole("menu")).toBeNull();
  expect(trigger).toHaveFocus();
});

// A reveal that fails says so, and the session's copy stays downloadable: it
// used to mark the card "Download unavailable" and disable Download.
test("a failed reveal leaves the download available", async () => {
  vi.spyOn(window, "fetch").mockResolvedValue(
    new Response(null, { status: 500 }),
  );
  render(
    <ArtifactCard
      inline
      artifact={{
        ...artifact,
        revealUrl: "/foxxycode/sessions/s1/artifacts/artifact-1/reveal",
      }}
    />,
  );
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("menuitem", { name: "Reveal on server" }));

  expect(
    await screen.findByText("Could not reveal the file on the server"),
  ).toBeVisible();
  expect(screen.queryByText("Download unavailable")).toBeNull();
  fireEvent.click(trigger);
  expect(screen.getByRole("menuitem", { name: "Download" })).toBeEnabled();
});

test("Tab closes the menu and leaves the focus moving on from the trigger", () => {
  render(<ArtifactCard inline artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  fireEvent.click(trigger);
  const tab = fireEvent.keyDown(screen.getByRole("menu"), { key: "Tab" });
  // Not prevented: the browser's own Tab then moves on from the trigger.
  expect(tab).toBe(true);
  expect(screen.queryByRole("menu")).toBeNull();
  expect(trigger).toHaveFocus();
});

test("places the menu under the trigger, above it at the window's foot, and inside the window", () => {
  const view = { width: 390, height: 720 };
  const menu = { width: 200, height: 238 };
  // Under the trigger, right edges together.
  expect(
    placeArtifactMenu({ top: 100, right: 360, bottom: 128 }, menu, view),
  ).toEqual({ left: 160, top: 134 });
  // A trigger at a phone's left edge keeps the menu on the screen.
  expect(
    placeArtifactMenu({ top: 100, right: 130, bottom: 128 }, menu, view),
  ).toEqual({ left: 8, top: 134 });
  // No room below: above the trigger.
  expect(
    placeArtifactMenu({ top: 650, right: 360, bottom: 678 }, menu, view),
  ).toEqual({ left: 160, top: 406 });
  // No room on either side: as low as the window allows.
  expect(
    placeArtifactMenu({ top: 120, right: 360, bottom: 148 }, menu, {
      width: 390,
      height: 300,
    }),
  ).toEqual({
    left: 160,
    top: 54,
  });
});

test("downloads a local artifact through a direct anchor without fetching", () => {
  const fetchSpy = vi.spyOn(window, "fetch");
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});
  render(
    <ToolCallMessage
      toolCallId="share-local"
      title="share_file"
      status="completed"
      artifacts={[artifact]}
    />,
  );

  fireEvent.click(
    screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
  );
  fireEvent.click(screen.getByRole("menuitem", { name: "Download" }));
  expect(click).toHaveBeenCalledTimes(1);
  expect(fetchSpy).not.toHaveBeenCalled();
});

test("artifact actions escape the clipped card and close on Escape", () => {
  render(<ArtifactCard artifact={artifact} />);
  fireEvent.click(
    screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
  );
  const menu = screen.getByRole("menu");
  expect(
    screen.getByTestId("tool-artifact-card-artifact-1"),
  ).not.toContainElement(menu);
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.queryByRole("menu")).toBeNull();
});

test("a second pointer click on artifact actions closes the menu", () => {
  render(<ArtifactCard artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  fireEvent.mouseDown(trigger);
  fireEvent.click(trigger);
  expect(screen.getByRole("menu")).toBeVisible();
  fireEvent.mouseDown(trigger);
  fireEvent.click(trigger);
  expect(screen.queryByRole("menu")).toBeNull();
});

// A resize used to close the menu outright; now it re-places the menu from its
// trigger, and closes it only once the trigger cannot be seen any more. The
// resize path runs the target-less follow, never the outside-press check that
// reads `event.target` as a Node: the resize event's target is the Window, and
// a `contains(Window)` there throws before anything is re-placed or closed.
test("resizing the window re-places the menu without treating Window as a Node", () => {
  render(<ArtifactCard artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  const at = (top: number) =>
    vi
      .spyOn(trigger, "getBoundingClientRect")
      .mockReturnValue(
        DOMRect.fromRect({ x: 272, y: top, width: 28, height: 28 }),
      );

  at(100);
  fireEvent.click(trigger);
  expect(screen.getByRole("menu")).toHaveStyle({ top: "134px" });

  at(400);
  fireEvent.resize(window);
  expect(screen.getByRole("menu")).toBeVisible();
  expect(screen.getByRole("menu")).toHaveStyle({ top: "434px" });
});

test("resizing the window closes the menu once its trigger is out of sight", () => {
  render(<ArtifactCard artifact={artifact} />);
  const trigger = screen.getByRole("button", {
    name: "Actions for release-notes.pdf",
  });
  const rect = vi
    .spyOn(trigger, "getBoundingClientRect")
    .mockReturnValue(
      DOMRect.fromRect({ x: 272, y: 300, width: 28, height: 28 }),
    );
  fireEvent.click(trigger);
  expect(screen.getByRole("menu")).toBeVisible();

  rect.mockReturnValue(
    DOMRect.fromRect({ x: 272, y: -80, width: 28, height: 28 }),
  );
  fireEvent.resize(window);
  expect(screen.queryByRole("menu")).toBeNull();
});

test("a shared file below a tool call can mention its source in the composer", () => {
  const onMentionArtifact = vi.fn();
  render(
    <ToolCallMessage
      toolCallId="share-mention"
      title="share_file"
      status="completed"
      artifacts={[{ ...artifact, relativePath: "reports/release-notes.pdf" }]}
      onMentionArtifact={onMentionArtifact}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
  );
  fireEvent.click(screen.getByRole("menuitem", { name: "Mention source" }));
  expect(onMentionArtifact).toHaveBeenCalledWith("reports/release-notes.pdf");
});

test.each([
  ["Copy name", "release-notes.pdf"],
  ["Copy relative path", "reports/release-notes.pdf"],
  ["Copy absolute path", "C:\\QA folder\\reports\\release-notes.pdf"],
])(
  "%s falls back when the embedded browser denies Clipboard API writes",
  async (label, expected) => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: vi
          .fn()
          .mockRejectedValue(
            new DOMException("Write permission denied", "NotAllowedError"),
          ),
      },
    });
    const execCommand = vi.fn(() => {
      expect((document.activeElement as HTMLTextAreaElement).value).toBe(
        expected,
      );
      return true;
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });
    render(
      <ArtifactCard
        artifact={{
          ...artifact,
          relativePath: "reports/release-notes.pdf",
          sourcePath: "C:\\QA folder\\reports\\release-notes.pdf",
        }}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: label }));
    await waitFor(() => expect(execCommand).toHaveBeenCalledWith("copy"));
    expect(document.querySelector("textarea")).toBeNull();
  },
);

test.each(["local", "remote"] as const)(
  "reveals an artifact with POST in the %s environment",
  async (mode) => {
    if (mode === "remote")
      setEnv({
        mode,
        baseUrl: "https://remote.example",
        token: "reveal-test-token",
      });
    const revealUrl = `${artifact.url}/reveal`;
    const fetchSpy = vi
      .spyOn(window, "fetch")
      .mockImplementation(
        async (_url, init) =>
          new Response(null, { status: init?.method === "POST" ? 204 : 405 }),
      );
    render(<ArtifactCard artifact={{ ...artifact, revealUrl }} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Reveal on server" }));
    await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(1));
    const [url, init] = fetchSpy.mock.calls[0]!;
    expect(url).toBe(
      mode === "remote" ? `https://remote.example${revealUrl}` : revealUrl,
    );
    expect(init?.method).toBe("POST");
    if (mode === "remote")
      expect(new Headers(init?.headers).get("Authorization")).toBe(
        "Bearer reveal-test-token",
      );
    expect(
      screen.queryByText("Could not reveal the file on the server"),
    ).toBeNull();
  },
);

test("downloads a relay-mounted artifact through the environment request and releases its blob URL", async () => {
  setEnv({
    mode: "remote",
    baseUrl: "https://relay.example/swarm/nodes/nas02",
    token: "secret",
  });
  const fetchSpy = vi
    .spyOn(window, "fetch")
    .mockResolvedValue(new Response(new Blob(["artifact"]), { status: 200 }));
  const createObjectURL = vi.fn(() => "blob:artifact-download");
  const revokeObjectURL = vi.fn();
  vi.stubGlobal("URL", { ...URL, createObjectURL, revokeObjectURL });
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});

  render(
    <ToolCallMessage
      toolCallId="share-4"
      title="share_file"
      status="completed"
      artifacts={[artifact]}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Actions for release-notes.pdf" }),
  );
  fireEvent.click(screen.getByRole("menuitem", { name: "Download" }));

  await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(1));
  expect(fetchSpy.mock.calls[0]?.[0]).toBe(
    "https://relay.example/swarm/nodes/nas02" + artifact.url,
  );
  expect(
    new Headers(fetchSpy.mock.calls[0]?.[1]?.headers).get("Authorization"),
  ).toBe("Bearer secret");
  expect(click).toHaveBeenCalledTimes(1);
  await waitFor(() =>
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:artifact-download"),
  );
});
