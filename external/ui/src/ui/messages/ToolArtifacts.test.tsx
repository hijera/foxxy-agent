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
