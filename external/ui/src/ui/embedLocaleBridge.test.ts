import { afterEach, beforeEach, expect, test } from "vitest";
import {
  installEmbedLocaleBridge,
  LOCALE_MESSAGE_TYPE,
} from "./embedLocaleBridge";
import { setLocale } from "./i18n/i18n";

const realParent = window.parent;
let uninstall: (() => void) | null = null;

function fakeParent(): { posts: unknown[]; parent: Pick<Window, "postMessage"> } {
  const posts: unknown[] = [];
  const parent = {
    postMessage: (msg: unknown) => {
      posts.push(msg);
    },
  } as unknown as Window;
  Object.defineProperty(window, "parent", {
    value: parent,
    configurable: true,
  });
  return { posts, parent };
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.history.replaceState({}, "", "/");
  delete document.documentElement.dataset.embed;
  setLocale("en");
});

afterEach(() => {
  uninstall?.();
  uninstall = null;
  Object.defineProperty(window, "parent", {
    value: realParent,
    configurable: true,
  });
  setLocale("en");
});

test("posts the effective locale to the parent frame inside an editor embed", () => {
  document.documentElement.dataset.embed = "intellij";
  const { posts } = fakeParent();

  uninstall = installEmbedLocaleBridge();
  expect(uninstall).not.toBeNull();

  setLocale("ru");
  expect(posts).toEqual([{ type: LOCALE_MESSAGE_TYPE, locale: "ru" }]);
});

test("does nothing in a plain browser session (no embed marker)", () => {
  const { posts } = fakeParent();

  uninstall = installEmbedLocaleBridge();
  expect(uninstall).toBeNull();

  setLocale("ru");
  expect(posts).toEqual([]);
});

test("does nothing when the SPA is the top-level document (JCEF)", () => {
  document.documentElement.dataset.embed = "intellij";
  // window.parent === window: IntelliJ's JCEF case.
  uninstall = installEmbedLocaleBridge();
  expect(uninstall).toBeNull();
});
