import React from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SessionChangesCard } from "./SessionChangesCard";
import { setSessionChangesEnabled } from "../chat/sessionChangesConfig";
import type { SessionChanges } from "./types";

const CHANGES: SessionChanges = {
  sessionId: "s1",
  files: [
    {
      path: "src/02-game.js",
      status: "modified",
      additions: 8,
      deletions: 0,
      binary: false,
      truncated: false,
    },
    {
      path: "index.html",
      status: "added",
      additions: 134,
      deletions: 2,
      binary: false,
      truncated: false,
    },
  ],
  totals: { files: 2, additions: 142, deletions: 2 },
};

const EMPTY: SessionChanges = {
  sessionId: "s1",
  files: [],
  totals: { files: 0, additions: 0, deletions: 0 },
};

function jsonResponse(body: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => body,
  } as unknown as Response;
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  setSessionChangesEnabled(true);
  window.sessionStorage.clear();
  fetchMock = vi.fn(async () => jsonResponse(CHANGES));
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  setSessionChangesEnabled(true);
});

test("the card summarises the session and lists its files", async () => {
  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={() => {}}
      onOpenViewer={() => {}}
    />,
  );
  const card = await screen.findByTestId("session-changes-card");
  expect(card).toHaveTextContent("2 files changed");
  expect(card).toHaveTextContent("+142");
  expect(card).toHaveTextContent("−2");
  expect(screen.getByTestId("changes-row-src/02-game.js")).toHaveTextContent("+8");
});

test("a session that changed nothing renders no card", async () => {
  fetchMock.mockResolvedValue(jsonResponse(EMPTY));
  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={() => {}}
      onOpenViewer={() => {}}
    />,
  );
  await waitFor(() => expect(fetchMock).toHaveBeenCalled());
  expect(screen.queryByTestId("session-changes-card")).toBeNull();
});

test("the preference hides the card and stops it fetching", async () => {
  setSessionChangesEnabled(false);
  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={() => {}}
      onOpenViewer={() => {}}
    />,
  );
  await waitFor(() => {
    expect(screen.queryByTestId("session-changes-card")).toBeNull();
  });
  expect(fetchMock).not.toHaveBeenCalled();
});

test("outside an editor the review opens the full review window", async () => {
  const onOpenReview = vi.fn();
  const onOpenViewer = vi.fn();
  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={onOpenReview}
      onOpenViewer={onOpenViewer}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-review"));
  expect(onOpenViewer).toHaveBeenCalledTimes(1);
  // The drawer answers about one file, so Review must not open it.
  expect(onOpenReview).not.toHaveBeenCalled();
  // Only the change set was fetched; no IDE hand-off was attempted.
  expect(
    fetchMock.mock.calls.some((c) => String(c[0]).includes("open-in-ide")),
  ).toBe(false);
});

test("the summary opens the full review window too", async () => {
  const onOpenViewer = vi.fn();
  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={() => {}}
      onOpenViewer={onOpenViewer}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-card-open"));
  expect(onOpenViewer).toHaveBeenCalledTimes(1);
});

test("clicking a row opens that file in the side drawer", async () => {
  const onOpenReview = vi.fn();
  const onOpenViewer = vi.fn();
  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={onOpenReview}
      onOpenViewer={onOpenViewer}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-row-index.html"));
  expect(onOpenReview).toHaveBeenCalledWith("index.html");
  expect(onOpenViewer).not.toHaveBeenCalled();
});

test("inside an editor embed the review still opens the review window", async () => {
  window.sessionStorage.setItem("foxxycode.embed", "intellij");
  const onOpenReview = vi.fn();
  const onOpenViewer = vi.fn();

  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={onOpenReview}
      onOpenViewer={onOpenViewer}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-review"));

  // Review is about the change set, and the window is the only surface that
  // shows a change set. Handing it to the IDE gave one file at a time instead,
  // which is what a row click is for.
  expect(onOpenViewer).toHaveBeenCalledTimes(1);
  expect(onOpenReview).not.toHaveBeenCalled();
  expect(
    fetchMock.mock.calls.some((c) => String(c[0]).includes("open-in-ide")),
  ).toBe(false);
});

test("inside an editor embed the summary opens the review window too", async () => {
  window.sessionStorage.setItem("foxxycode.embed", "intellij");
  const onOpenViewer = vi.fn();

  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={() => {}}
      onOpenViewer={onOpenViewer}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-card-open"));
  expect(onOpenViewer).toHaveBeenCalledTimes(1);
});

test("a row click inside an editor embed names the file to the plugin", async () => {
  window.sessionStorage.setItem("foxxycode.embed", "intellij");
  fetchMock.mockImplementation(async (input: unknown) => {
    if (String(input).includes("open-in-ide")) {
      return jsonResponse({ delivered: true });
    }
    return jsonResponse(CHANGES);
  });

  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={() => {}}
      onOpenViewer={() => {}}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-row-index.html"));

  await waitFor(() => {
    const call = fetchMock.mock.calls.find((c) =>
      String(c[0]).includes("open-in-ide"),
    );
    expect(call).toBeTruthy();
    const body = String((call![1] as RequestInit | undefined)?.body ?? "");
    expect(JSON.parse(body)).toEqual({ path: "index.html" });
  });
});

test("with no plugin listening a row falls back to the in-app drawer", async () => {
  window.sessionStorage.setItem("foxxycode.embed", "vscode");
  const onOpenReview = vi.fn();
  fetchMock.mockImplementation(async (input: unknown) => {
    if (String(input).includes("open-in-ide")) {
      return jsonResponse({ delivered: false });
    }
    return jsonResponse(CHANGES);
  });

  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={onOpenReview}
      onOpenViewer={() => {}}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-row-index.html"));
  await waitFor(() => expect(onOpenReview).toHaveBeenCalledWith("index.html"));
});

test("undo asks before rewriting the workspace", async () => {
  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={() => {}}
      onOpenViewer={() => {}}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-revert"));
  expect(
    fetchMock.mock.calls.some((c) => String(c[0]).includes("/revert")),
  ).toBe(false);

  fireEvent.click(screen.getByTestId("changes-revert-confirm"));
  await waitFor(() => {
    expect(
      fetchMock.mock.calls.some((c) => String(c[0]).includes("/revert")),
    ).toBe(true);
  });
});

test("declining the confirmation leaves the workspace alone", async () => {
  render(
    <SessionChangesCard
      sessionId="s1"
      generating={false}
      onOpenReview={() => {}}
      onOpenViewer={() => {}}
    />,
  );
  fireEvent.click(await screen.findByTestId("changes-revert"));
  fireEvent.click(screen.getByTestId("changes-revert-cancel"));
  expect(screen.getByTestId("changes-revert")).toBeTruthy();
  expect(
    fetchMock.mock.calls.some((c) => String(c[0]).includes("/revert")),
  ).toBe(false);
});
