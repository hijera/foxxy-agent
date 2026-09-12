import { expect, test, vi } from "vitest";
import {
  classifyPastedText,
  pasteChipLiteralKey,
  pasteChipToken,
  shouldAttemptPasteClassify,
  PASTE_CLASSIFY_MAX_BYTES,
} from "./pasteChip";

test("shouldAttemptPasteClassify requires an editor embed", () => {
  expect(shouldAttemptPasteClassify("line one\nline two", false)).toBe(false);
  expect(shouldAttemptPasteClassify("line one\nline two", true)).toBe(true);
});

test("shouldAttemptPasteClassify gates weak and oversize text", () => {
  expect(shouldAttemptPasteClassify("x := 1", true)).toBe(false);
  expect(shouldAttemptPasteClassify("a single line long enough to try", true)).toBe(true);
  expect(shouldAttemptPasteClassify("   \n  ", true)).toBe(false);
  expect(shouldAttemptPasteClassify("x".repeat(PASTE_CLASSIFY_MAX_BYTES + 1), true)).toBe(false);
});

test("pasteChipToken builds mention tokens", () => {
  expect(pasteChipToken({ kind: "file", pathRel: "Dockerfile", startLine: 21, endLine: 31 })).toBe(
    "@Dockerfile:21-31",
  );
  expect(pasteChipToken({ kind: "terminal", terminalName: "dev" })).toBe("@terminal:dev");
  expect(pasteChipToken({ kind: "terminal", terminalName: "" })).toBe("@terminal");
  expect(pasteChipToken({ kind: "none" })).toBe(null);
  expect(pasteChipToken({ kind: "file", pathRel: "f.go", startLine: 0, endLine: 3 })).toBe(null);
});

test("pasteChipLiteralKey is path:start-end", () => {
  expect(pasteChipLiteralKey("a/b.go", 2, 4)).toBe("a/b.go:2-4");
});

test("classifyPastedText returns the parsed file result", async () => {
  const fetchImpl = vi.fn(async () =>
    new Response(JSON.stringify({ kind: "file", pathRel: "f.go", startLine: 2, endLine: 4 }), {
      status: 200,
    }),
  ) as unknown as typeof fetch;
  const got = await classifyPastedText("some\ntext", "sess_1", fetchImpl);
  expect(got).toEqual({ kind: "file", pathRel: "f.go", startLine: 2, endLine: 4 });
});

test("classifyPastedText sends the session header", async () => {
  const calls: RequestInit[] = [];
  const fetchImpl = (async (_url: unknown, init?: RequestInit) => {
    calls.push(init ?? {});
    return new Response(JSON.stringify({ kind: "none" }), { status: 200 });
  }) as unknown as typeof fetch;
  await classifyPastedText("some\ntext", "sess_9", fetchImpl);
  expect((calls[0]!.headers as Record<string, string>)["X-FoxxyCode-Session-ID"]).toBe("sess_9");
});

test("classifyPastedText degrades to none on failure", async () => {
  const rejecting = vi.fn(async () => {
    throw new Error("boom");
  }) as unknown as typeof fetch;
  expect(await classifyPastedText("some\ntext", "", rejecting)).toEqual({ kind: "none" });

  const badStatus = vi.fn(async () => new Response("nope", { status: 500 })) as unknown as typeof fetch;
  expect(await classifyPastedText("some\ntext", "", badStatus)).toEqual({ kind: "none" });

  const badBody = vi.fn(async () => new Response("{}", { status: 200 })) as unknown as typeof fetch;
  expect(await classifyPastedText("some\ntext", "", badBody)).toEqual({ kind: "none" });
});
