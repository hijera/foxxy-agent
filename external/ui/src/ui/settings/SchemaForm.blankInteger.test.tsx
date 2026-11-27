import React from "react";
import { afterEach, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SchemaForm, type JsonSchema } from "./SchemaForm";

afterEach(cleanup);

// An integer field whose schema default is null, such as
// models[].max_screenshots: a blank field means "unset" - the saved document
// must drop the key so the server-side inheritance applies - while 0 is a real
// value ("zero disables the cap") and must survive the save.

const sectionSchema = {
  type: "object",
  title: "Model",
  properties: {
    model: { type: "string", title: "Model" },
    max_screenshots: {
      type: "integer",
      title: "Maximum screenshots per request",
      description:
        "User image attachments also count. Zero disables the limit.",
      minimum: 0,
      default: null,
    },
  },
  required: ["model"],
} as unknown as JsonSchema;

function Harness(props: {
  initial: Record<string, unknown>;
  changes: Record<string, unknown>[];
}) {
  const [doc, setDoc] = React.useState(props.initial);
  return (
    <SchemaForm
      schema={sectionSchema}
      value={doc}
      onChange={(next) => {
        props.changes.push(next);
        setDoc(next);
      }}
    />
  );
}

function budgetInput(): HTMLInputElement {
  return screen.getByRole("spinbutton", {
    name: "Maximum screenshots per request",
  }) as HTMLInputElement;
}

test("an unset blank-null integer renders a blank field, not 0", () => {
  const changes: Record<string, unknown>[] = [];
  render(<Harness initial={{ model: "fake/model" }} changes={changes} />);
  expect(budgetInput().value).toBe("");
});

test("clearing the field drops the key so the value stays unset", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ model: "fake/model", max_screenshots: 4 }}
      changes={changes}
    />,
  );
  expect(budgetInput().value).toBe("4");

  fireEvent.change(budgetInput(), { target: { value: "" } });
  expect(changes.length).toBeGreaterThan(0);
  const last = changes[changes.length - 1]!;
  expect(last["max_screenshots"]).toBeUndefined();
  // The save serializes the document with JSON.stringify, which drops
  // undefined keys: the PUT body omits the field and the model inherits the
  // browser-wide limit.
  expect(JSON.stringify(last)).not.toContain("max_screenshots");
});

test("an explicit 0 is a real value and survives the save", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ model: "fake/model", max_screenshots: 4 }}
      changes={changes}
    />,
  );

  fireEvent.change(budgetInput(), { target: { value: "0" } });
  const last = changes[changes.length - 1]!;
  expect(last["max_screenshots"]).toBe(0);
  expect(JSON.stringify(last)).toContain('"max_screenshots":0');
});
