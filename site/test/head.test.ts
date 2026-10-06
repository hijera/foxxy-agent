// @vitest-environment node
//
// The three page heads are hand written and duplicated, which is how they
// drifted apart: every page promised a summary_large_image card, none named a
// twitter:image, and each declared two of the four icons the embedded SPA
// declares. Worse, the preview they did name was a stale export still carrying
// the upstream product's name - see internal/brand for the guard on that half.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const ROOT = resolve(__dirname, "..");

const PAGES = [
  { file: "index.html", assets: "../docs/assets" },
  { file: "compare/index.html", assets: "../../docs/assets" },
  { file: "changelog/index.html", assets: "../../docs/assets" },
] as const;

const SOCIAL = "https://hijera.github.io/foxxy-agent/assets/foxxycode-logo-social-1280x640.png";

function head(file: string): string {
  const html = readFileSync(resolve(ROOT, file), "utf8");
  const end = html.indexOf("</head>");
  expect(end, `${file} has no </head>`).toBeGreaterThan(0);
  return html.slice(0, end);
}

describe.each(PAGES)("$file", ({ file, assets }) => {
  // The SVG icon must be foxxycode-favicon.svg rather than one of the fixed-plate
  // marks: it is the only file whose plate follows prefers-color-scheme, and the
  // browser tab strip is the one surface no theme of ours reaches.
  it("declares every icon the SPA declares", () => {
    const markup = head(file);
    for (const link of [
      `<link rel="icon" type="image/svg+xml" href="${assets}/foxxycode-favicon.svg" />`,
      `<link rel="icon" type="image/png" sizes="32x32" href="${assets}/favicon-32.png" />`,
      `<link rel="icon" sizes="any" href="${assets}/favicon.ico" />`,
      `<link rel="apple-touch-icon" href="${assets}/apple-touch-icon.png" />`,
    ]) {
      expect(markup).toContain(link);
    }
  });

  it("names the social preview for both og and twitter", () => {
    const markup = head(file);
    expect(markup).toContain(`<meta property="og:image" content="${SOCIAL}" />`);
    expect(markup).toContain(`<meta name="twitter:card" content="summary_large_image" />`);
    // A large-image card with no image of its own is the bug this catches.
    expect(markup).toContain(`<meta name="twitter:image" content="${SOCIAL}" />`);
  });
});
