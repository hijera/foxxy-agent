import { describe, expect, it } from "vitest";
import { setLocale } from "../i18n/i18n";
import {
  permissionOptionLabel,
  programGrantFromOptionName,
} from "./permissionOptionLabel";

const opt = (optionId: string, name: string) => ({
  optionId,
  name,
  kind: "allow_once" as const,
});

describe("permissionOptionLabel", () => {
  it("translates the fixed options by id, not by their English prose", () => {
    setLocale("ru");
    try {
      expect(permissionOptionLabel(opt("allow", "Allow"))).toBe("Разрешить");
      expect(permissionOptionLabel(opt("allow_always", "Allow always"))).toBe(
        "Всегда разрешать",
      );
      expect(permissionOptionLabel(opt("reject", "Reject"))).toBe("Отклонить");
    } finally {
      setLocale("en");
    }
  });

  // The backend names the grant it would store and does not send it separately,
  // so the program has to survive the round trip through the label.
  it("keeps the program name when translating the program-wide option", () => {
    setLocale("ru");
    try {
      expect(
        permissionOptionLabel(
          opt("allow_always_program", "Always allow git status"),
        ),
      ).toContain("git status");
    } finally {
      setLocale("en");
    }
  });

  it("falls back to the backend text when the label does not parse", () => {
    expect(
      permissionOptionLabel(opt("allow_always_program", "Разрешить всё")),
    ).toBe("Разрешить всё");
  });

  it("passes an unknown option through untouched", () => {
    expect(permissionOptionLabel(opt("something_new", "Do the thing"))).toBe(
      "Do the thing",
    );
  });
});

describe("programGrantFromOptionName", () => {
  it("extracts bare programs and multiplexer subcommands", () => {
    expect(programGrantFromOptionName("Always allow curl")).toBe("curl");
    expect(programGrantFromOptionName("Always allow git status")).toBe(
      "git status",
    );
    expect(programGrantFromOptionName("  Always allow make  ")).toBe("make");
  });

  it("returns nothing for a name it does not recognise", () => {
    expect(programGrantFromOptionName("Allow always")).toBe("");
    expect(programGrantFromOptionName("")).toBe("");
  });
});
