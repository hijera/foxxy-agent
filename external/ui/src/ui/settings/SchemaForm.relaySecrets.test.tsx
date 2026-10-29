import React from "react";
import { afterEach, beforeEach, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SchemaForm, type JsonSchema } from "./SchemaForm";
import { initLocale } from "../i18n/i18n";

afterEach(cleanup);
beforeEach(() => initLocale("en"));

// A credential the configuration document never sends back (DESIGN.md,
// Settings drawer, "Secrets the server never sends back"): a relay's tokens
// arrive empty with a sibling flag saying whether one is set (issue #401).
const relaySchema = {
  type: "object",
  "x-foxxycode-property-order": ["name", "auth_token", "pairing_tokens"],
  properties: {
    name: { type: "string", title: "Name" },
    auth_token: {
      type: "string",
      title: "Client token",
      writeOnly: true,
      "x-foxxycode-configured": "auth_configured",
    },
    pairing_tokens: {
      type: "array",
      title: "Pairing tokens",
      writeOnly: true,
      "x-foxxycode-configured": "pairing_configured",
      items: { type: "string" },
    },
  },
} as unknown as JsonSchema;

function Harness(props: {
  initial: Record<string, unknown>;
  changes: Record<string, unknown>[];
}) {
  const [doc, setDoc] = React.useState<Record<string, unknown>>(props.initial);
  return (
    <SchemaForm
      schema={relaySchema}
      value={doc}
      onChange={(next) => {
        props.changes.push(next);
        setDoc(next);
      }}
    />
  );
}

test("a secret that is set is an empty password field that says so", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ name: "office", auth_configured: true }}
      changes={changes}
    />,
  );
  const field = screen.getByLabelText("Client token") as HTMLInputElement;
  expect(field.type).toBe("password");
  expect(field.value).toBe("");
  expect(field.placeholder).toBe("Set. Leave empty to keep it");
  fireEvent.change(field, { target: { value: "rotated" } });
  expect(changes.at(-1)?.auth_token).toBe("rotated");
});

test("a secret that is not set says that instead", () => {
  render(<Harness initial={{ name: "office" }} changes={[]} />);
  const field = screen.getByLabelText("Client token") as HTMLInputElement;
  expect(field.placeholder).toBe("Not set");
});

test("a list of secrets counts what is set, and its entries are hidden", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ name: "office", pairing_configured: 2, pairing_tokens: [] }}
      changes={changes}
    />,
  );
  expect(screen.getByTestId("settings-secret-list-state").textContent).toBe(
    "2 are set and not shown. Values entered here replace them all.",
  );
  fireEvent.click(screen.getByRole("button", { name: /add/i }));
  const entry = screen.getByLabelText("Pairing tokens 1") as HTMLInputElement;
  expect(entry.type).toBe("password");
});
