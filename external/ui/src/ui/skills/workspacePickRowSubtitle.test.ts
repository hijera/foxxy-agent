import { expect, test } from "vitest";
import { workspacePickRowSubtitle } from "./workspacePickRowSubtitle";

test("root file has empty subtitle", () => {
  expect(
    workspacePickRowSubtitle({
      path_rel: "Cargo.toml",
    }),
  ).toBe("");
});

test("nested file shows parent folder only", () => {
  expect(
    workspacePickRowSubtitle({
      path_rel: "src/ui/App.tsx",
    }),
  ).toBe("src/ui/");
});

test("directory at workspace root has empty subtitle", () => {
  expect(
    workspacePickRowSubtitle({
      path_rel: "pkg/",
    }),
  ).toBe("");
});

test("nested directory shows parent folder only", () => {
  expect(
    workspacePickRowSubtitle({
      path_rel: "src/ui/dialogs/",
    }),
  ).toBe("src/ui/");
});
