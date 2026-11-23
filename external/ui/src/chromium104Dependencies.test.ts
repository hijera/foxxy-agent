import { expect, test } from "vitest";
import { chromium104DependencyCode } from "./compat/chromium104Dependencies";

const id = "/project/node_modules/@braintree/sanitize-url/dist/index.js";
const source = "return URL.canParse(url);";

test.each([
  ["https://example.com/a", true],
  ["http://[::1]/", true],
  ["https://пример.рф/путь", true],
  ["/relative", false],
  ["https://", false],
  ["http://[broken]/", false],
])("the sanitizer still validates absolute URLs: %s", (url, valid) => {
  const code = chromium104DependencyCode(source, id)!;
  expect(code).not.toContain("URL.canParse");
  expect(new Function("url", code)(url)).toBe(valid);
});

test("handles Windows dependency paths and leaves application code alone", () => {
  expect(chromium104DependencyCode(source, id.replaceAll("/", "\\"))).not.toContain("URL.canParse");
  expect(chromium104DependencyCode(source, "/project/src/app.ts")).toBeNull();
});

test.each([
  "/project/node_modules/chevrotain/lib/src/parse/grammar/checks.js",
  "/project/node_modules/@mermaid-js/parser/dist/chunks/parser.mjs",
  "/project/node_modules/mermaid/dist/chunks/usecaseDiagram.mjs",
])("the grammar parser groups productions without Object.groupBy: %s", (id) => {
  const code = chromium104DependencyCode(
    "return Object.groupBy(items, keyOf);",
    id,
  )!;
  expect(code).not.toContain("Object.groupBy");
  const symbol = Symbol("production");
  const items = ["__proto__", "constructor", "__proto__", symbol];
  const keys: number[] = [];
  const grouped = new Function("items", "keyOf", code)(items, (item: PropertyKey, index: number) => {
    keys.push(index);
    return item;
  });
  expect(Object.getPrototypeOf(grouped)).toBeNull();
  expect(grouped.__proto__).toEqual(["__proto__", "__proto__"]);
  expect(grouped.constructor).toEqual(["constructor"]);
  expect(grouped[symbol]).toEqual([symbol]);
  expect(keys).toEqual([0, 1, 2, 3]);
});
