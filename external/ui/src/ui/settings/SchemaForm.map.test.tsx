import React from "react";
import { afterEach, expect, test } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { SchemaForm, defaultForSchema, type JsonSchema } from "./SchemaForm";

afterEach(cleanup);

// A map of plain values (DESIGN.md "Section form layout", Maps): an object
// schema with no properties of its own and a string schema for every value,
// such as tools.http_request.default_headers. Each entry is a row of two bare
// inputs - the name, then the value - beside its trash, with Add under the rows.

const mapSchema = {
  type: "object",
  title: "Default headers",
  description: "Headers every request sends.",
  additionalProperties: { type: "string" },
} as unknown as JsonSchema;

const sectionSchema = {
  type: "object",
  "x-foxxycode-property-order": ["allowlist", "default_headers"],
  properties: {
    allowlist: { type: "array", title: "Allowlist", items: { type: "string" } },
    default_headers: mapSchema,
  },
} as unknown as JsonSchema;

/** Holds the document the form edits and reports every change it makes. */
function Harness(props: {
  initial: Record<string, unknown>;
  changes: Record<string, unknown>[];
  replace?: { current: ((doc: Record<string, unknown>) => void) | null };
}) {
  const [doc, setDoc] = React.useState<Record<string, unknown>>(props.initial);
  if (props.replace) props.replace.current = setDoc;
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

function nameInputs(): HTMLInputElement[] {
  return screen.getAllByRole("textbox", { name: /Default headers \d+: name/ });
}

function valueInputs(): HTMLInputElement[] {
  return screen.getAllByRole("textbox", { name: /Default headers \d+: value/ });
}

/** The map's own fieldset: the list beside it has an Add of its own. */
function mapField() {
  const legend = screen
    .getAllByText("Default headers")
    .find((el) => el.closest("legend"));
  return within(legend!.closest("fieldset")!);
}

function lastHeaders(changes: Record<string, unknown>[]): unknown {
  return changes[changes.length - 1]?.default_headers;
}

test("a map renders one row per entry, in the document's order, inside a fieldset of its own", () => {
  const changes: Record<string, unknown>[] = [];
  const { container } = render(
    <Harness
      initial={{
        allowlist: [],
        default_headers: { "User-Agent": "Mozilla/5.0", Accept: "text/html" },
      }}
      changes={changes}
    />,
  );
  const legend = [...container.querySelectorAll("legend")].map((l) =>
    l.textContent?.trim(),
  );
  expect(legend).toContain("Default headers");
  expect(nameInputs().map((i) => i.value)).toEqual(["User-Agent", "Accept"]);
  expect(valueInputs().map((i) => i.value)).toEqual([
    "Mozilla/5.0",
    "text/html",
  ]);
  expect(container.querySelectorAll(".settings-map-entry")).toHaveLength(2);
  expect(changes).toHaveLength(0);
});

test("editing a value or a name writes the map with the entry in its place", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ default_headers: { "User-Agent": "a", Accept: "b" } }}
      changes={changes}
    />,
  );
  fireEvent.change(valueInputs()[0]!, { target: { value: "Mozilla/5.0" } });
  expect(lastHeaders(changes)).toEqual({
    "User-Agent": "Mozilla/5.0",
    Accept: "b",
  });
  fireEvent.change(nameInputs()[1]!, { target: { value: "X-Client" } });
  const headers = lastHeaders(changes) as Record<string, string>;
  expect(Object.entries(headers)).toEqual([
    ["User-Agent", "Mozilla/5.0"],
    ["X-Client", "b"],
  ]);
});

test("Add opens an empty row that stays on screen and out of the document until it has a name", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ default_headers: { Accept: "b" } }}
      changes={changes}
    />,
  );
  fireEvent.click(mapField().getByRole("button", { name: "Add" }));
  expect(nameInputs()).toHaveLength(2);
  // The document is what it was, so the form is not edited yet: a copy of the
  // configuration that arrives now may still replace it.
  fireEvent.change(valueInputs()[1]!, { target: { value: "foxxycode-lab" } });
  expect(nameInputs()).toHaveLength(2);
  expect(changes).toHaveLength(0);
  fireEvent.change(nameInputs()[1]!, { target: { value: " X-Client " } });
  expect(lastHeaders(changes)).toEqual({
    Accept: "b",
    "X-Client": "foxxycode-lab",
  });
});

test("a name the object prototype answers to is a key like any other", () => {
  const changes: Record<string, unknown>[] = [];
  render(<Harness initial={{}} changes={changes} />);
  fireEvent.click(mapField().getByRole("button", { name: "Add" }));
  fireEvent.change(nameInputs()[0]!, { target: { value: "__proto__" } });
  fireEvent.change(valueInputs()[0]!, { target: { value: "x" } });
  const headers = lastHeaders(changes) as Record<string, string>;
  expect(Object.keys(headers)).toEqual(["__proto__"]);
  expect(JSON.stringify(headers)).toBe('{"__proto__":"x"}');
});

test("a removed row takes its focus with it instead of handing it to the next row's trash", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ default_headers: { "User-Agent": "a", Accept: "b" } }}
      changes={changes}
    />,
  );
  const first = mapField().getAllByRole("button", { name: "Remove" })[0]!;
  first.focus();
  fireEvent.click(first);
  expect(nameInputs().map((i) => i.value)).toEqual(["Accept"]);
  const left = mapField().getByRole("button", { name: "Remove" });
  expect(document.activeElement).not.toBe(left);
});

test("a name with an empty value is kept: it leaves that header out", () => {
  const changes: Record<string, unknown>[] = [];
  render(<Harness initial={{}} changes={changes} />);
  fireEvent.click(mapField().getByRole("button", { name: "Add" }));
  fireEvent.change(nameInputs()[0]!, { target: { value: "User-Agent" } });
  expect(lastHeaders(changes)).toEqual({ "User-Agent": "" });
});

test("the trash removes an entry, and the last one leaves an empty map", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ default_headers: { "User-Agent": "a", Accept: "b" } }}
      changes={changes}
    />,
  );
  fireEvent.click(mapField().getAllByRole("button", { name: "Remove" })[0]!);
  expect(lastHeaders(changes)).toEqual({ Accept: "b" });
  expect(nameInputs().map((i) => i.value)).toEqual(["Accept"]);
  fireEvent.click(mapField().getByRole("button", { name: "Remove" }));
  expect(lastHeaders(changes)).toEqual({});
  expect(
    screen.queryAllByRole("textbox", { name: /Default headers/ }),
  ).toHaveLength(0);
});

test("two rows with one name keep the last value and both stay on screen", () => {
  const changes: Record<string, unknown>[] = [];
  render(
    <Harness
      initial={{ default_headers: { "User-Agent": "a", Accept: "b" } }}
      changes={changes}
    />,
  );
  fireEvent.change(nameInputs()[1]!, { target: { value: "User-Agent" } });
  expect(lastHeaders(changes)).toEqual({ "User-Agent": "b" });
  expect(nameInputs().map((i) => i.value)).toEqual([
    "User-Agent",
    "User-Agent",
  ]);
});

test("a document replaced from outside the form replaces the rows", () => {
  const changes: Record<string, unknown>[] = [];
  const replace: { current: ((doc: Record<string, unknown>) => void) | null } =
    {
      current: null,
    };
  render(
    <Harness
      initial={{ default_headers: { Accept: "b" } }}
      changes={changes}
      replace={replace}
    />,
  );
  fireEvent.click(mapField().getByRole("button", { name: "Add" }));
  expect(nameInputs()).toHaveLength(2);
  act(() => {
    replace.current!({ default_headers: { "User-Agent": "reloaded" } });
  });
  expect(nameInputs().map((i) => i.value)).toEqual(["User-Agent"]);
  expect(valueInputs().map((i) => i.value)).toEqual(["reloaded"]);
});

test("the default of a map is an empty map, never a string", () => {
  expect(defaultForSchema(mapSchema)).toEqual({});
  const withoutMap = defaultForSchema({
    type: "object",
    properties: { default_headers: mapSchema },
  } as unknown as JsonSchema);
  expect(withoutMap).toEqual({ default_headers: {} });
});
