import { afterEach, beforeEach, expect, test } from "vitest";
import {
  hostMentionPaths,
  installEmbedHostBridge,
  INSERT_FILE_MENTION_MESSAGE_TYPE,
  MAX_HOST_MENTION_PATHS,
} from "./embedHostBridge";
import { subscribeFileMention } from "./skills/fileMentionBus";

const realParent = window.parent;
let uninstall: (() => void) | null = null;
let unsubscribe: (() => void) | null = null;

/** Replaces window.parent with a distinct object so `ev.source` can be compared against it. */
function fakeParent(): Window {
  const parent = { postMessage: () => {} } as unknown as Window;
  Object.defineProperty(window, "parent", { value: parent, configurable: true });
  return parent;
}

function received(): string[] {
  const got: string[] = [];
  unsubscribe = subscribeFileMention((p) => got.push(p));
  return got;
}

function post(source: unknown, data: unknown): void {
  const ev = new MessageEvent("message", { data, source: source as Window });
  window.dispatchEvent(ev);
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.history.replaceState({}, "", "/");
  delete document.documentElement.dataset.embed;
});

afterEach(() => {
  uninstall?.();
  uninstall = null;
  unsubscribe?.();
  unsubscribe = null;
  Object.defineProperty(window, "parent", { value: realParent, configurable: true });
});

test("forwards paths from the parent frame to the file-mention bus, in order", () => {
  document.documentElement.dataset.embed = "vscode";
  const parent = fakeParent();
  const got = received();

  uninstall = installEmbedHostBridge();
  expect(uninstall).not.toBeNull();

  post(parent, { type: INSERT_FILE_MENTION_MESSAGE_TYPE, paths: ["src/a.ts", "README.md"] });
  expect(got).toEqual(["src/a.ts", "README.md"]);
});

test("ignores messages that do not come from the parent frame", () => {
  document.documentElement.dataset.embed = "vscode";
  fakeParent();
  const got = received();
  uninstall = installEmbedHostBridge();

  post(window, { type: INSERT_FILE_MENTION_MESSAGE_TYPE, paths: ["src/a.ts"] });
  post(null, { type: INSERT_FILE_MENTION_MESSAGE_TYPE, paths: ["src/a.ts"] });
  expect(got).toEqual([]);
});

test("ignores other message types and malformed payloads", () => {
  document.documentElement.dataset.embed = "vscode";
  const parent = fakeParent();
  const got = received();
  uninstall = installEmbedHostBridge();

  post(parent, { type: "foxxycode:locale", locale: "ru" });
  post(parent, { type: INSERT_FILE_MENTION_MESSAGE_TYPE, paths: "src/a.ts" });
  post(parent, { type: INSERT_FILE_MENTION_MESSAGE_TYPE });
  post(parent, "src/a.ts");
  post(parent, null);
  expect(got).toEqual([]);
});

test("is a no-op outside an editor embed and when the SPA is top-level", () => {
  fakeParent();
  expect(installEmbedHostBridge()).toBeNull();

  document.documentElement.dataset.embed = "intellij";
  Object.defineProperty(window, "parent", { value: window, configurable: true });
  expect(installEmbedHostBridge()).toBeNull();
});

test("hostMentionPaths trims, drops non-strings and blanks, and caps the batch", () => {
  const many = Array.from({ length: MAX_HOST_MENTION_PATHS + 5 }, (_, i) => `f${i}.ts`);
  expect(
    hostMentionPaths({
      type: INSERT_FILE_MENTION_MESSAGE_TYPE,
      paths: ["  src/a.ts ", "", "   ", 42, null, "x".repeat(5000), ...many],
    }),
  ).toEqual(["src/a.ts", ...many.slice(0, MAX_HOST_MENTION_PATHS - 1)]);
  expect(hostMentionPaths(undefined)).toEqual([]);
  expect(hostMentionPaths({ type: "other", paths: ["a"] })).toEqual([]);
});
