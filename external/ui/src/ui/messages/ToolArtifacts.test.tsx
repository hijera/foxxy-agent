import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

import { setEnv } from "../env/remoteEnv";
import { parseToolArtifacts } from "../chat/toolArtifacts";
import { ToolCallMessage } from "./ToolCallMessage";
import { ArtifactCard } from "./ToolArtifactCards";

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
  expect(parseToolArtifacts([artifact, { ...artifact, id: "gone", url: "" }])).toEqual([
    artifact,
    { ...artifact, id: "gone", url: undefined },
  ]);
  expect(parseToolArtifacts([{ ...artifact, sha256: "not-a-digest" }])).toEqual([]);
});
test("drops artifact URLs that are not root-relative server routes", () => {
  const hostile = {
    ...artifact,
    url: "javascript:alert(1)",
    previewUrl: "https://evil.example/pixel",
    revealUrl: "//evil.example/reveal",
  };
  expect(parseToolArtifacts([hostile])).toEqual([{ ...unavailableArtifact, previewUrl: undefined, revealUrl: undefined }]);
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
  expect(screen.queryByRole("button", { name: "Download release-notes.pdf" })).toBeNull();
  expect(screen.getByRole("button", { name: "Actions for release-notes.pdf" })).toBeVisible();
  expect(screen.getByTestId("tool-details-share-1")).not.toHaveAttribute("open");
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
  fireEvent.click(screen.getByRole("button", { name: "Actions for release-notes.pdf" }));
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
  expect(screen.getByRole("img", { name: "release-overview.png" })).toHaveAttribute(
    "src",
    "/foxxycode/sessions/s1/artifacts/artifact-1/preview",
  );
  expect(container.querySelector(".inline-artifact-extension")?.parentElement).toHaveClass(
    "tool-artifact-card",
  );
});

test("downloads a local artifact through a direct anchor without fetching", () => {
  const fetchSpy = vi.spyOn(window, "fetch");
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  render(
    <ToolCallMessage
      toolCallId="share-local"
      title="share_file"
      status="completed"
      artifacts={[artifact]}
    />,
  );

  fireEvent.click(screen.getByRole("button", { name: "Actions for release-notes.pdf" }));
  fireEvent.click(screen.getByRole("menuitem", { name: "Download" }));
  expect(click).toHaveBeenCalledTimes(1);
  expect(fetchSpy).not.toHaveBeenCalled();
});

test("artifact actions escape the clipped card and close on Escape", () => {
  render(<ArtifactCard artifact={artifact} />);
  fireEvent.click(screen.getByRole("button", { name: "Actions for release-notes.pdf" }));
  const menu = screen.getByRole("menu");
  expect(screen.getByTestId("tool-artifact-card-artifact-1")).not.toContainElement(menu);
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.queryByRole("menu")).toBeNull();
});

test("a second pointer click on artifact actions closes the menu", () => {
  render(<ArtifactCard artifact={artifact} />);
  const trigger = screen.getByRole("button", { name: "Actions for release-notes.pdf" });
  fireEvent.mouseDown(trigger);
  fireEvent.click(trigger);
  expect(screen.getByRole("menu")).toBeVisible();
  fireEvent.mouseDown(trigger);
  fireEvent.click(trigger);
  expect(screen.queryByRole("menu")).toBeNull();
});

test("resizing the window closes artifact actions without treating Window as a Node", () => {
  render(<ArtifactCard artifact={artifact} />);
  fireEvent.click(screen.getByRole("button", { name: "Actions for release-notes.pdf" }));
  fireEvent.resize(window);
  expect(screen.queryByRole("menu")).toBeNull();
});

test("a shared file below a tool call can mention its source in the composer", () => {
  const onMentionArtifact = vi.fn();
  render(<ToolCallMessage toolCallId="share-mention" title="share_file" status="completed"
    artifacts={[{ ...artifact, relativePath: "reports/release-notes.pdf" }]} onMentionArtifact={onMentionArtifact} />);
  fireEvent.click(screen.getByRole("button", { name: "Actions for release-notes.pdf" }));
  fireEvent.click(screen.getByRole("menuitem", { name: "Mention source" }));
  expect(onMentionArtifact).toHaveBeenCalledWith("reports/release-notes.pdf");
});

test.each([
  ["Copy name", "release-notes.pdf"],
  ["Copy relative path", "reports/release-notes.pdf"],
  ["Copy absolute path", "C:\\QA folder\\reports\\release-notes.pdf"],
])("%s falls back when the embedded browser denies Clipboard API writes", async (label, expected) => {
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new DOMException("Write permission denied", "NotAllowedError")) } });
  const execCommand = vi.fn(() => {
    expect((document.activeElement as HTMLTextAreaElement).value).toBe(expected);
    return true;
  });
  Object.defineProperty(document, "execCommand", { configurable: true, value: execCommand });
  render(<ArtifactCard artifact={{ ...artifact, relativePath: "reports/release-notes.pdf", sourcePath: "C:\\QA folder\\reports\\release-notes.pdf" }} />);
  fireEvent.click(screen.getByRole("button", { name: "Actions for release-notes.pdf" }));
  fireEvent.click(screen.getByRole("menuitem", { name: label }));
  await waitFor(() => expect(execCommand).toHaveBeenCalledWith("copy"));
  expect(document.querySelector("textarea")).toBeNull();
});

test.each(["local", "remote"] as const)("reveals an artifact with POST in the %s environment", async (mode) => {
  if (mode === "remote") setEnv({ mode, baseUrl: "https://remote.example", token: "reveal-test-token" });
  const revealUrl = `${artifact.url}/reveal`;
  const fetchSpy = vi.spyOn(window, "fetch").mockImplementation(async (_url, init) =>
    new Response(null, { status: init?.method === "POST" ? 204 : 405 }),
  );
  render(<ArtifactCard artifact={{ ...artifact, revealUrl }} />);
  fireEvent.click(screen.getByRole("button", { name: "Actions for release-notes.pdf" }));
  fireEvent.click(screen.getByRole("menuitem", { name: "Reveal on server" }));
  await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(1));
  const [url, init] = fetchSpy.mock.calls[0]!;
  expect(url).toBe(mode === "remote" ? `https://remote.example${revealUrl}` : revealUrl);
  expect(init?.method).toBe("POST");
  if (mode === "remote") expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer reveal-test-token");
  expect(screen.queryByText("Could not reveal the file on the server")).toBeNull();
});

test("downloads a remote artifact through the environment request and releases its blob URL", async () => {
  setEnv({ mode: "remote", baseUrl: "https://remote.example", token: "secret" });
  const fetchSpy = vi.spyOn(window, "fetch").mockResolvedValue(
    new Response(new Blob(["artifact"]), { status: 200 }),
  );
  const createObjectURL = vi.fn(() => "blob:artifact-download");
  const revokeObjectURL = vi.fn();
  vi.stubGlobal("URL", { ...URL, createObjectURL, revokeObjectURL });
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

  render(
    <ToolCallMessage
      toolCallId="share-4"
      title="share_file"
      status="completed"
      artifacts={[artifact]}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Actions for release-notes.pdf" }));
  fireEvent.click(screen.getByRole("menuitem", { name: "Download" }));

  await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(1));
  expect(fetchSpy.mock.calls[0]?.[0]).toBe("https://remote.example" + artifact.url);
  expect(new Headers(fetchSpy.mock.calls[0]?.[1]?.headers).get("Authorization")).toBe("Bearer secret");
  expect(click).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith("blob:artifact-download"));
});
