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

describe("http_request grants", () => {
  it("names the address or the origin the grant covers", () => {
    const url = {
      optionId: "allow_always_url",
      name: "Always allow https://api.x.dev/v1/items",
    };
    const origin = {
      optionId: "allow_always_origin",
      name: "Always allow https://api.x.dev",
    };
    expect(permissionOptionLabel(url)).toBe(
      "Always allow https://api.x.dev/v1/items",
    );
    setLocale("ru");
    try {
      expect(permissionOptionLabel(url)).toBe(
        "Всегда разрешать https://api.x.dev/v1/items",
      );
      expect(permissionOptionLabel(origin)).toBe(
        "Всегда разрешать https://api.x.dev",
      );
    } finally {
      setLocale("en");
    }
  });

  it("falls back to the backend's own text for a name it cannot read", () => {
    setLocale("ru");
    try {
      expect(
        permissionOptionLabel({
          optionId: "allow_always_origin",
          name: "Allow everything",
        }),
      ).toBe("Allow everything");
    } finally {
      setLocale("en");
    }
  });
});
