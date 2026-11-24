/** Adapt renderer dependencies to the JCEF browser baseline at build time. */
export function groupBy104<T>(items: Iterable<T>, keyOf: (item: T, index: number) => PropertyKey) {
  if (typeof keyOf !== "function") throw new TypeError("groupBy requires a callback");
  const groups: Record<PropertyKey, T[]> = Object.create(null);
  let index = 0;
  for (const item of items) {
    const key = keyOf(item, index++);
    (groups[key] ??= []).push(item);
  }
  return groups;
}

export function canParseUrl104(url: string, base?: string): boolean {
  try {
    new URL(url, base);
    return true;
  } catch {
    return false;
  }
}

export function chromium104DependencyCode(code: string, id: string): string | null {
  const file = id.replaceAll("\\", "/");
  if (/\/node_modules\/(?:chevrotain|@mermaid-js\/parser|mermaid)\/.*\.[cm]?js$/.test(file)) {
    // Chevrotain groups grammar productions by a string key. Keep its parser
    // on the same baseline without adding Object.groupBy to the page globals.
    return code.replaceAll(
      "Object.groupBy",
      `(${groupBy104.toString()})`,
    );
  }
  if (!/\/node_modules\/@braintree\/sanitize-url\/dist\/index\.js$/.test(file)) return null;
  // URL.canParse is the non-throwing form of the URL constructor. The sanitizer
  // only passes an absolute URL, so preserve that check without a global shim
  // or an older sanitizer that would lose subsequent security fixes.
  return code.replaceAll(
    "URL.canParse",
    `(${canParseUrl104.toString()})`,
  );
}
