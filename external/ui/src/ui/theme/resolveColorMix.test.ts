import { describe, expect, it } from "vitest";
import postcss from "postcss";
// eslint-disable-next-line
// @ts-ignore -- plain .mjs module without type declarations
import resolveColorMix from "../../../postcss-resolve-color-mix.mjs";

async function run(css: string): Promise<string> {
  const result = await postcss([resolveColorMix()]).process(css, {
    from: undefined,
  });
  return result.css;
}

const TWO_THEMES = `
:root,
[data-theme="dark"] {
  --text: #ffffff;
  --blend: transparent;
}
[data-theme="light"] {
  --text: #18181b;
  --blend: #ffffff;
}
`;

describe("postcss-resolve-color-mix", () => {
  it("resolves a transparent mix to the exact rgba alpha", async () => {
    const out = await run(`${TWO_THEMES}
.a { background: color-mix(in srgb, #ffffff 8%, transparent); }`);
    expect(out).toContain("background: rgba(255, 255, 255, 0.08)");
    expect(out).not.toContain("color-mix(");
  });

  it("emits per-theme --cmix vars when themes diverge", async () => {
    const out = await run(`${TWO_THEMES}
.a { color: color-mix(in srgb, var(--text) 50%, var(--blend)); }`);
    const varName = out.match(/var\((--cmix-[a-z0-9]+)\)/)?.[1];
    expect(varName).toBeTruthy();
    // dark: white 50% over transparent -> rgba(255,255,255,0.5)
    expect(out).toMatch(
      new RegExp(
        `\\[data-theme="dark"\\][^}]*${varName}: rgba\\(255, 255, 255, 0\\.5\\)`,
        "s",
      ),
    );
    // light: #18181b 50% + #ffffff 50% -> opaque mid grey
    expect(out).toMatch(
      new RegExp(`\\[data-theme="light"\\][^}]*${varName}: #8c8c8d`, "s"),
    );
    expect(out).not.toContain("color-mix(");
  });

  it("inlines a literal when all themes agree", async () => {
    const out = await run(`${TWO_THEMES}
.a { color: color-mix(in srgb, #000000 50%, #ffffff); }`);
    expect(out).toContain("color: #808080");
    expect(out).not.toContain("--cmix-");
  });

  it("normalizes percentages per css-color-5 (sum < 100 scales alpha)", async () => {
    // 30% + 30% = 60 -> equal weights, alpha multiplier 0.6
    const out = await run(`${TWO_THEMES}
.a { color: color-mix(in srgb, #ffffff 30%, #000000 30%); }`);
    expect(out).toContain("color: rgba(128, 128, 128, 0.6)");
  });

  it("derives the missing percentage as the complement", async () => {
    const out = await run(`${TWO_THEMES}
.a { color: color-mix(in srgb, #ffffff, #000000 25%); }`);
    // 75% white + 25% black
    expect(out).toContain("color: #bfbfbf");
  });

  it("handles multiple mixes in one declaration (scrollbar-color)", async () => {
    const out = await run(`${TWO_THEMES}
.a { scrollbar-color: color-mix(in srgb, #ffffff 22%, transparent) transparent; }`);
    expect(out).toContain(
      "scrollbar-color: rgba(255, 255, 255, 0.22) transparent",
    );
  });

  it("handles multiline formatted expressions", async () => {
    const out = await run(`${TWO_THEMES}
.a {
  background: color-mix(
    in srgb,
    #ffffff 70%,
    rgba(0, 0, 0, 0.16)
  );
}`);
    expect(out).not.toContain("color-mix(");
    expect(out).toMatch(/background: rgba\(\d+, \d+, \d+, 0\.\d+\)/);
  });

  it("resolves mixes inside theme-block custom properties in place", async () => {
    const out = await run(`
:root,
[data-theme="dark"] {
  --text: #ffffff;
  --panel-border: color-mix(in srgb, var(--text) 12%, transparent);
}
[data-theme="light"] {
  --text: #18181b;
  --panel-border: rgba(0, 0, 0, 0.08);
}
.a { border-color: var(--panel-border); }`);
    expect(out).toContain("--panel-border: rgba(255, 255, 255, 0.12)");
    expect(out).not.toContain("color-mix(");
  });

  it("dedupes identical expressions into one --cmix var", async () => {
    const out = await run(`${TWO_THEMES}
.a { color: color-mix(in srgb, var(--text) 72%, var(--blend)); }
.b { color: color-mix(in srgb, var(--text) 72%, var(--blend)); }`);
    const matches = [...out.matchAll(/var\((--cmix-[a-z0-9]+)\)/g)].map(
      (m) => m[1],
    );
    expect(matches).toHaveLength(2);
    expect(matches[0]).toBe(matches[1]);
    // Emitted once per theme block, not per usage.
    const emitted = [...out.matchAll(/--cmix-[a-z0-9]+:/g)];
    expect(emitted).toHaveLength(2);
  });

  it("produces a stable hash for equivalent whitespace variants", async () => {
    const a = await run(`${TWO_THEMES}
.a { color: color-mix(in srgb, var(--text) 72%, var(--blend)); }`);
    const b = await run(`${TWO_THEMES}
.a { color: color-mix( in srgb,  var(--text)   72% , var(--blend) ); }`);
    const nameA = a.match(/var\((--cmix-[a-z0-9]+)\)/)?.[1];
    const nameB = b.match(/var\((--cmix-[a-z0-9]+)\)/)?.[1];
    expect(nameA).toBeTruthy();
    expect(nameA).toBe(nameB);
  });

  it("fails the build on non-srgb interpolation", async () => {
    await expect(
      run(`${TWO_THEMES}
.a { color: color-mix(in oklch, #ffffff 50%, #000000); }`),
    ).rejects.toThrow(/in srgb/);
  });

  it("fails the build on an unknown custom property", async () => {
    await expect(
      run(`${TWO_THEMES}
.a { color: color-mix(in srgb, var(--nope) 50%, #000000); }`),
    ).rejects.toThrow(/unknown custom property --nope/);
  });

  it("fails the build on unsupported color functions", async () => {
    await expect(
      run(`${TWO_THEMES}
.a { color: color-mix(in srgb, hsl(0, 50%, 50%) 50%, #000000); }`),
    ).rejects.toThrow(/unresolvable/);
  });

  it("uses the var() fallback when the property is undefined everywhere", async () => {
    const out = await run(`${TWO_THEMES}
.a { color: color-mix(in srgb, var(--missing, #ffffff) 8%, transparent); }`);
    expect(out).toContain("color: rgba(255, 255, 255, 0.08)");
  });

  it("resolves var chains (var pointing at var)", async () => {
    const out = await run(`
:root,
[data-theme="dark"] {
  --accent: #9333ea;
  --ring: var(--accent);
}
[data-theme="light"] {
  --accent: #9333ea;
}
.a { color: color-mix(in srgb, var(--ring) 50%, transparent); }`);
    expect(out).not.toContain("color-mix(");
    expect(out).toContain("rgba(147, 51, 234, 0.5)");
  });
});
