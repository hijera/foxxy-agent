import { beforeEach, expect, test } from "vitest";
import {
  bootstrapEmbedFlag,
  hostResolvesFileDrops,
  isEditorEmbed,
  readEmbedFromUrl,
} from "./embedShell";

beforeEach(() => {
  window.sessionStorage.clear();
  window.history.replaceState({}, "", "/");
  delete document.documentElement.dataset.embed;
});

test("readEmbedFromUrl returns the ?embed= value", () => {
  window.history.replaceState({}, "", "/?embed=intellij");
  expect(readEmbedFromUrl()).toBe("intellij");
});

test("readEmbedFromUrl returns the value alongside other params", () => {
  window.history.replaceState({}, "", "/?theme=dark&lang=ru&embed=intellij");
  expect(readEmbedFromUrl()).toBe("intellij");
});

test("readEmbedFromUrl is empty without the marker", () => {
  window.history.replaceState({}, "", "/?lang=ru");
  expect(readEmbedFromUrl()).toBe("");
});

test("desktop marker is not an editor embed", () => {
  window.history.replaceState({}, "", "/?desktop=1");
  expect(readEmbedFromUrl()).toBe("");
  expect(bootstrapEmbedFlag()).toBe(false);
  expect(isEditorEmbed()).toBe(false);
});

test("bootstrap latches the flag so isEditorEmbed survives hash navigation", () => {
  window.history.replaceState({}, "", "/?embed=intellij");
  expect(bootstrapEmbedFlag()).toBe(true);
  // SPA hash routing drops the query string on later navigations.
  window.history.replaceState({}, "", "/#/chat");
  expect(readEmbedFromUrl()).toBe("");
  expect(isEditorEmbed()).toBe(true);
});

test("isEditorEmbed falls back to the data-embed DOM marker", () => {
  document.documentElement.dataset.embed = "intellij";
  expect(isEditorEmbed()).toBe(true);
});

test("isEditorEmbed is false in a plain browser session", () => {
  expect(isEditorEmbed()).toBe(false);
  expect(bootstrapEmbedFlag()).toBe(false);
});

test("the vscode embed is an editor embed that resolves its own file drops", () => {
  window.history.replaceState({}, "", "/?embed=vscode");
  expect(bootstrapEmbedFlag()).toBe(true);
  expect(isEditorEmbed()).toBe(true);
  // Only JCEF hands the plugin the dropped paths; the VS Code iframe gets a
  // text/uri-list and relativizes it through the backend itself.
  expect(hostResolvesFileDrops()).toBe(false);
});

test("only the intellij embed leaves file drops to the host", () => {
  document.documentElement.dataset.embed = "intellij";
  expect(hostResolvesFileDrops()).toBe(true);
});
