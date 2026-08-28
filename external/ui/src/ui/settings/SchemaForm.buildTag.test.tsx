import React from "react";
import { afterEach, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SchemaForm, type JsonSchema } from "./SchemaForm";
import { setLocale } from "../i18n/i18n";

afterEach(() => {
  cleanup();
  setLocale("en");
});

// Mirrors the browser section the Go schema endpoint serves: it always names the
// build tag it needs, and the serving process adds x-foxxycode-build-tag-missing
// when its own binary was compiled without that tag.
function browserSection(missing: boolean): JsonSchema {
  return {
    type: "object",
    properties: {
      browser: {
        type: "object",
        title: "Browser tool",
        description:
          "Interactive browser automation tool (requires the browser build tag; drives a local Chrome/Chromium via chromedp).",
        properties: {
          enabled: {
            type: "boolean",
            title: "Enabled",
            description: "Turns on the interactive browser tools.",
          },
          executable_path: {
            type: "string",
            title: "Browser executable",
            description: "Optional path to a Chrome/Chromium binary.",
          },
        },
        "x-foxxycode-requires-build-tag": "browser",
        ...(missing ? { "x-foxxycode-build-tag-missing": true } : {}),
      },
    },
  } as unknown as JsonSchema;
}

function renderSection(missing: boolean, value: Record<string, unknown> = {}) {
  function Harness() {
    const [doc, setDoc] = React.useState<Record<string, unknown>>(value);
    return (
      <SchemaForm schema={browserSection(missing)} value={doc} onChange={setDoc} />
    );
  }
  return render(<Harness />);
}

// The whole point of the change: the section stays visible so the feature is
// discoverable, but nothing in it can be switched on, because this binary cannot
// run it whatever the config says.
test("a section whose build tag is missing is shown but cannot be edited", () => {
  renderSection(true);

  expect(screen.getByText("Browser tool")).toBeTruthy();
  const toggle = screen.getByRole("switch", { name: "Enabled" }) as HTMLButtonElement;
  expect(toggle.disabled).toBe(true);
  expect(
    (screen.getByLabelText("Browser executable") as HTMLInputElement).disabled,
  ).toBe(true);

  // Clicking it must not change the document.
  fireEvent.click(toggle);
  expect(
    screen.getByRole("switch", { name: "Enabled" }).getAttribute("aria-checked"),
  ).toBe("false");
});

// A dead toggle with no explanation is worse than no toggle: the notice has to name
// the tag, so the user knows what to rebuild with.
test("the notice names the build tag to rebuild with", () => {
  renderSection(true);
  const notice = screen.getByTestId("settings-build-tag-notice");
  expect(notice.textContent).toContain("browser");
});

test("the Russian notice also names the tag", () => {
  setLocale("ru");
  renderSection(true);
  const notice = screen.getByTestId("settings-build-tag-notice");
  expect(notice.textContent).toContain("browser");
  // Translated, not the English fallback.
  expect(notice.textContent).toMatch(/[а-яА-Я]/);
});

// A binary that does have the tag must render an ordinary, editable section — the
// requires-build-tag marker alone must never disable anything.
test("a section whose build tag is present stays editable", () => {
  renderSection(false);
  const toggle = screen.getByRole("switch", { name: "Enabled" }) as HTMLButtonElement;
  expect(toggle.disabled).toBe(false);
  expect(screen.queryByTestId("settings-build-tag-notice")).toBeNull();

  fireEvent.click(toggle);
  expect(
    screen.getByRole("switch", { name: "Enabled" }).getAttribute("aria-checked"),
  ).toBe("true");
});

// An operator may already have browser.enabled: true in a YAML file shared with a
// full build. Showing it as off would be a lie; it is on, and simply inert here.
test("an already-enabled value is still displayed, just not editable", () => {
  renderSection(true, { browser: { enabled: true } });
  const toggle = screen.getByRole("switch", { name: "Enabled" }) as HTMLButtonElement;
  expect(toggle.getAttribute("aria-checked")).toBe("true");
  expect(toggle.disabled).toBe(true);
});

// SettingsSection hands each top-level section's SUB-schema to SchemaForm as its
// root — the tab heading already names the section. So the annotation arrives on
// the root schema, not on a nested field, and the root must honour it too. This is
// the shape the real settings screen produces; the nested cases above are not.
function rootSection(missing: boolean): JsonSchema {
  return {
    type: "object",
    title: "Browser tool",
    properties: {
      enabled: {
        type: "boolean",
        title: "Enabled",
        description: "Turns on the interactive browser tools.",
      },
      timeout_seconds: {
        type: "integer",
        title: "Action timeout (seconds)",
        description: "Per-action timeout.",
      },
    },
    "x-foxxycode-requires-build-tag": "browser",
    ...(missing ? { "x-foxxycode-build-tag-missing": true } : {}),
  } as unknown as JsonSchema;
}

test("a top-level section (the real settings tab shape) is disabled too", () => {
  function Harness() {
    const [doc, setDoc] = React.useState<Record<string, unknown>>({});
    return <SchemaForm schema={rootSection(true)} value={doc} onChange={setDoc} />;
  }
  render(<Harness />);

  const notice = screen.getByTestId("settings-build-tag-notice");
  expect(notice.textContent).toContain("browser");
  const toggle = screen.getByRole("switch", { name: "Enabled" }) as HTMLButtonElement;
  expect(toggle.disabled).toBe(true);
  expect(
    (screen.getByLabelText("Action timeout (seconds)") as HTMLInputElement).disabled,
  ).toBe(true);
});

test("a top-level section with its tag present stays editable", () => {
  function Harness() {
    const [doc, setDoc] = React.useState<Record<string, unknown>>({});
    return <SchemaForm schema={rootSection(false)} value={doc} onChange={setDoc} />;
  }
  render(<Harness />);

  expect(screen.queryByTestId("settings-build-tag-notice")).toBeNull();
  expect((screen.getByRole("switch", { name: "Enabled" }) as HTMLButtonElement).disabled).toBe(false);
});

// The root wrapper is a <div>, not a <fieldset>, so it gets no native disabling of
// descendants: every control type has to be wired explicitly or it stays clickable
// inside an otherwise dead section.
test("enum and array controls in a disabled section are inert too", () => {
  const schema = {
    type: "object",
    properties: {
      mode: {
        type: "string",
        title: "Mode",
        enum: ["a", "b"],
      },
      hosts: {
        type: "array",
        title: "Hosts",
        items: { type: "string", title: "Host" },
      },
    },
    "x-foxxycode-requires-build-tag": "browser",
    "x-foxxycode-build-tag-missing": true,
  } as unknown as JsonSchema;

  function Harness() {
    const [doc, setDoc] = React.useState<Record<string, unknown>>({ hosts: ["one"] });
    return <SchemaForm schema={schema} value={doc} onChange={setDoc} />;
  }
  render(<Harness />);

  expect((screen.getByLabelText("Mode") as HTMLInputElement).disabled).toBe(true);
  for (const b of screen.getAllByRole("button")) {
    expect((b as HTMLButtonElement).disabled).toBe(true);
  }
});
