import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { WorkspaceFolderModal } from "./WorkspaceFolderModal";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); document.documentElement.style.overflow = ""; });

test("the folder dialog locks page scrolling while open and restores it on close", () => {
  vi.stubGlobal("fetch", vi.fn(() => new Promise(() => {})));
  document.documentElement.style.overflow = "auto";
  const props = { startPath: "/", onClose: () => {}, onPick: () => {} };
  const view = render(<WorkspaceFolderModal {...props} open={false} />);
  expect(document.documentElement.style.overflow).toBe("auto");
  view.rerender(<WorkspaceFolderModal {...props} open />);
  expect(document.documentElement.style.overflow).toBe("hidden");
  view.rerender(<WorkspaceFolderModal {...props} open={false} />);
  expect(document.documentElement.style.overflow).toBe("auto");
  view.rerender(<WorkspaceFolderModal {...props} open />);
  view.unmount();
  expect(document.documentElement.style.overflow).toBe("auto");
});
