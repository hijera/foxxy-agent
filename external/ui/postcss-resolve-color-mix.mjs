/**
 * postcss-resolve-color-mix
 *
 * Resolves every `color-mix(in srgb, …)` expression in the stylesheet at build
 * time so the shipped CSS runs on Chromium 104 (JCEF in PhpStorm 2022.3.3),
 * which predates color-mix() support (Chromium 111).
 *
 * How it works:
 *  1. Collects custom-property values from the top-level theme blocks — rules
 *     whose selector list consists only of `:root` and/or `[data-theme="X"]`.
 *  2. Rewrites color-mix() inside those theme blocks to literal colors (the
 *     theme is known there).
 *  3. Rewrites every other color-mix() occurrence by computing the result for
 *     all themes: if the result is identical across themes it is inlined as a
 *     literal; otherwise the expression is replaced with `var(--cmix-<hash>)`
 *     and a `--cmix-<hash>: <value>` declaration is appended to every theme
 *     block.
 *
 * Constraints (enforced with a hard build failure listing all offenders):
 *  - only `in srgb` interpolation;
 *  - colors limited to hex, rgb()/rgba(), `transparent`, `white`, `black`,
 *    and `var(--name)` chains that resolve to one of those per theme.
 *
 * Trade-off: theme variables consumed by mixes are frozen at build time.
 * Mutating e.g. `--text` from JS at runtime will not propagate into the
 * precomputed mixed colors. No such mutation exists in this codebase.
 */

const THEME_ATTR_RE = /^\[data-theme="([a-z-]+)"\]$/;

const NAMED_COLORS = {
  transparent: { r: 0, g: 0, b: 0, a: 0 },
  white: { r: 255, g: 255, b: 255, a: 1 },
  black: { r: 0, g: 0, b: 0, a: 1 },
};

/** Split a string on top-level occurrences of a separator (ignoring parens). */
function splitTopLevel(input, sep) {
  const parts = [];
  let depth = 0;
  let current = "";
  for (const ch of input) {
    if (ch === "(") depth += 1;
    else if (ch === ")") depth -= 1;
    if (depth === 0 && ch === sep) {
      parts.push(current);
      current = "";
    } else {
      current += ch;
    }
  }
  parts.push(current);
  return parts.map((p) => p.trim()).filter((p) => p.length > 0);
}

/** Split on top-level whitespace (ignoring parens). */
function splitTopLevelWs(input) {
  const parts = [];
  let depth = 0;
  let current = "";
  for (const ch of input) {
    if (ch === "(") depth += 1;
    else if (ch === ")") depth -= 1;
    if (depth === 0 && /\s/.test(ch)) {
      if (current) parts.push(current);
      current = "";
    } else {
      current += ch;
    }
  }
  if (current) parts.push(current);
  return parts;
}

function parseHex(token) {
  const hex = token.slice(1);
  const expand = (s) => parseInt(s.length === 1 ? s + s : s, 16);
  if (hex.length === 3 || hex.length === 4) {
    return {
      r: expand(hex[0]),
      g: expand(hex[1]),
      b: expand(hex[2]),
      a: hex.length === 4 ? expand(hex[3]) / 255 : 1,
    };
  }
  if (hex.length === 6 || hex.length === 8) {
    return {
      r: expand(hex.slice(0, 2)),
      g: expand(hex.slice(2, 4)),
      b: expand(hex.slice(4, 6)),
      a: hex.length === 8 ? expand(hex.slice(6, 8)) / 255 : 1,
    };
  }
  return null;
}

function parseRgbFunc(token) {
  const m = token.match(/^rgba?\((.*)\)$/is);
  if (!m) return null;
  const inner = m[1].trim();
  let nums;
  if (inner.includes(",")) {
    nums = inner.split(",").map((s) => s.trim());
  } else {
    // rgb(R G B / A) space syntax
    const [rgbPart, aPart] = inner.split("/").map((s) => s.trim());
    nums = rgbPart.split(/\s+/);
    if (aPart !== undefined) nums.push(aPart);
  }
  if (nums.length !== 3 && nums.length !== 4) return null;
  const chan = (s) =>
    s.endsWith("%") ? (parseFloat(s) / 100) * 255 : parseFloat(s);
  const alpha = (s) => (s.endsWith("%") ? parseFloat(s) / 100 : parseFloat(s));
  const r = chan(nums[0]);
  const g = chan(nums[1]);
  const b = chan(nums[2]);
  const a = nums.length === 4 ? alpha(nums[3]) : 1;
  if ([r, g, b, a].some((v) => Number.isNaN(v))) return null;
  return { r, g, b, a };
}

function formatColor({ r, g, b, a }) {
  const ri = Math.round(r);
  const gi = Math.round(g);
  const bi = Math.round(b);
  if (a >= 0.9995) {
    const h = (v) => v.toString(16).padStart(2, "0");
    return `#${h(ri)}${h(gi)}${h(bi)}`;
  }
  if (a <= 0.0005) {
    return "rgba(0, 0, 0, 0)";
  }
  const af = parseFloat(a.toFixed(4));
  return `rgba(${ri}, ${gi}, ${bi}, ${af})`;
}

/** Canonical form of an expression: insignificant whitespace stripped. */
function normalizeExpr(expr) {
  return expr
    .replace(/\s+/g, " ")
    .replace(/\s*([(),])\s*/g, "$1")
    .trim();
}

/** FNV-1a 32-bit hash, base36 — stable var-name suffix for an expression. */
function hashExpr(normalized) {
  let h = 0x811c9dc5;
  for (let i = 0; i < normalized.length; i += 1) {
    h ^= normalized.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0).toString(36);
}

/** Extract every balanced `color-mix( … )` expression from a value string. */
function extractColorMixExprs(value) {
  const out = [];
  const lower = value.toLowerCase();
  let from = 0;
  for (;;) {
    const idx = lower.indexOf("color-mix(", from);
    if (idx === -1) break;
    let depth = 0;
    let end = -1;
    for (let i = idx + "color-mix".length; i < value.length; i += 1) {
      if (value[i] === "(") depth += 1;
      else if (value[i] === ")") {
        depth -= 1;
        if (depth === 0) {
          end = i;
          break;
        }
      }
    }
    if (end === -1) return null; // unbalanced — caller reports error
    out.push(value.slice(idx, end + 1));
    from = end + 1;
  }
  return out;
}

export default function resolveColorMix() {
  return {
    postcssPlugin: "resolve-color-mix",
    OnceExit(root) {
      const errors = [];
      const fail = (node, expr, reason) => {
        const src = node.source && node.source.start;
        const where = src ? `line ${src.line}` : "unknown line";
        const sel =
          node.parent && node.parent.selector
            ? node.parent.selector.replace(/\s+/g, " ")
            : "(no selector)";
        errors.push(`  ${where} | ${sel} | ${expr} — ${reason}`);
      };

      // ── 1. Collect theme variable maps from top-level theme blocks ──────
      const baseVars = new Map();
      const themeVars = new Map(); // theme -> Map(name -> raw value)
      const themeRules = new Map(); // theme -> first Rule for that theme
      const themeBlockRules = new Set(); // all rules recognized as theme blocks
      const themeIds = [];

      root.each((node) => {
        if (node.type !== "rule") return;
        const parts = node.selector.split(",").map((s) => s.trim());
        const matched = [];
        for (const p of parts) {
          if (p === ":root") {
            matched.push({ base: true });
          } else {
            const m = p.match(THEME_ATTR_RE);
            if (!m) return; // not a pure theme block (e.g. descendant selector)
            matched.push({ theme: m[1] });
          }
        }
        themeBlockRules.add(node);
        for (const m of matched) {
          if (m.theme && !themeIds.includes(m.theme)) themeIds.push(m.theme);
          if (m.theme && !themeRules.has(m.theme))
            themeRules.set(m.theme, node);
        }
        node.walkDecls(/^--/, (decl) => {
          for (const m of matched) {
            if (m.base) baseVars.set(decl.prop, decl.value);
            else {
              if (!themeVars.has(m.theme)) themeVars.set(m.theme, new Map());
              themeVars.get(m.theme).set(decl.prop, decl.value);
            }
          }
        });
      });

      if (themeIds.length === 0) {
        // Nothing that looks like a themed stylesheet (e.g. tiny test
        // fixtures without theme blocks): still resolve static mixes below
        // using a single pseudo-theme backed by :root only.
        themeIds.push("__base__");
      }

      const lookupVar = (theme, name) => {
        const tv = themeVars.get(theme);
        if (tv && tv.has(name)) return tv.get(name);
        if (baseVars.has(name)) return baseVars.get(name);
        return undefined;
      };

      // ── Color resolution ────────────────────────────────────────────────
      const resolveColorToken = (token, theme, node, stack) => {
        const t = token.trim();
        const lower = t.toLowerCase();
        if (lower in NAMED_COLORS) return { ...NAMED_COLORS[lower] };
        if (t.startsWith("#")) {
          const c = parseHex(t);
          if (c) return c;
          throw new Error(`invalid hex color "${t}"`);
        }
        if (/^rgba?\(/i.test(t)) {
          const c = parseRgbFunc(t);
          if (c) return c;
          throw new Error(`unsupported rgb() syntax "${t}"`);
        }
        const varMatch = t.match(/^var\(\s*(--[\w-]+)\s*(?:,([\s\S]*))?\)$/);
        if (varMatch) {
          const name = varMatch[1];
          if (stack.includes(name)) {
            throw new Error(`custom property cycle via ${name}`);
          }
          if (stack.length > 10) {
            throw new Error(`var() chain too deep at ${name}`);
          }
          let value = lookupVar(theme, name);
          if (value === undefined) {
            if (varMatch[2] !== undefined) value = varMatch[2].trim();
            else
              throw new Error(
                `unknown custom property ${name} for theme "${theme}"`,
              );
          }
          if (value.toLowerCase().includes("color-mix(")) {
            return evalColorMixExpr(extractSingle(value, node), theme, node, [
              ...stack,
              name,
            ]);
          }
          return resolveColorToken(value, theme, node, [...stack, name]);
        }
        throw new Error(
          `unsupported color "${t}" (allowed: hex, rgb()/rgba(), transparent, white, black, var())`,
        );
      };

      const extractSingle = (value, node) => {
        const exprs = extractColorMixExprs(value);
        if (!exprs || exprs.length !== 1 || exprs[0] !== value.trim()) {
          throw new Error(
            `expected a single color-mix() expression, got "${value}"`,
          );
        }
        return exprs[0];
      };

      /** Evaluate one full `color-mix(...)` expression for one theme. */
      const evalColorMixExpr = (expr, theme, node, stack = []) => {
        const inner = expr.slice("color-mix(".length, -1);
        const args = splitTopLevel(inner, ",");
        if (args.length !== 3) {
          throw new Error(`expected 3 arguments (interpolation, color, color)`);
        }
        if (!/^in\s+srgb$/i.test(args[0])) {
          throw new Error(
            `only "in srgb" interpolation is supported, got "${args[0]}"`,
          );
        }
        const parseComponent = (raw) => {
          const tokens = splitTopLevelWs(raw);
          let pct;
          const colorTokens = [];
          for (const tok of tokens) {
            const pm = tok.match(/^([\d.]+)%$/);
            if (pm) {
              if (pct !== undefined)
                throw new Error(`duplicate percentage in "${raw}"`);
              pct = parseFloat(pm[1]);
            } else {
              colorTokens.push(tok);
            }
          }
          if (colorTokens.length === 0)
            throw new Error(`missing color in "${raw}"`);
          const color = resolveColorToken(
            colorTokens.join(" "),
            theme,
            node,
            stack,
          );
          return { color, pct };
        };
        const c1 = parseComponent(args[1]);
        const c2 = parseComponent(args[2]);

        // Percentage normalization per css-color-5.
        let p1 = c1.pct;
        let p2 = c2.pct;
        if (p1 === undefined && p2 === undefined) {
          p1 = 50;
          p2 = 50;
        } else if (p1 === undefined) {
          p1 = 100 - p2;
        } else if (p2 === undefined) {
          p2 = 100 - p1;
        }
        const sum = p1 + p2;
        if (sum <= 0) throw new Error(`percentages sum to zero`);
        let alphaMult = 1;
        if (sum !== 100) {
          if (sum < 100) alphaMult = sum / 100;
          p1 /= sum;
          p2 /= sum;
        } else {
          p1 /= 100;
          p2 /= 100;
        }

        // Premultiplied-alpha interpolation in srgb.
        const a1 = c1.color.a;
        const a2 = c2.color.a;
        const a = a1 * p1 + a2 * p2;
        let r = 0;
        let g = 0;
        let b = 0;
        if (a > 0) {
          r = (c1.color.r * a1 * p1 + c2.color.r * a2 * p2) / a;
          g = (c1.color.g * a1 * p1 + c2.color.g * a2 * p2) / a;
          b = (c1.color.b * a1 * p1 + c2.color.b * a2 * p2) / a;
        }
        return { r, g, b, a: a * alphaMult };
      };

      // ── 2. Resolve mixes inside theme blocks in place ───────────────────
      for (const rule of themeBlockRules) {
        const parts = rule.selector.split(",").map((s) => s.trim());
        // The theme this block defines; shared `:root, [data-theme=X]` blocks
        // resolve with X's (== base) variables.
        const attr = parts.map((p) => p.match(THEME_ATTR_RE)).find(Boolean);
        const blockTheme = attr ? attr[1] : themeIds[0];
        rule.walkDecls((decl) => {
          if (!decl.value.toLowerCase().includes("color-mix(")) return;
          const exprs = extractColorMixExprs(decl.value);
          if (!exprs) {
            fail(decl, decl.value, "unbalanced parentheses");
            return;
          }
          let value = decl.value;
          for (const expr of exprs) {
            try {
              const resolved = formatColor(
                evalColorMixExpr(expr, blockTheme, decl),
              );
              value = value.replace(expr, resolved);
            } catch (e) {
              fail(decl, expr.replace(/\s+/g, " "), e.message);
            }
          }
          decl.value = value;
          if (decl.prop.startsWith("--")) {
            // Keep the maps in sync so later lookups see literals.
            for (const p of parts) {
              if (p === ":root") baseVars.set(decl.prop, value);
              const m = p.match(THEME_ATTR_RE);
              if (m) themeVars.get(m[1])?.set(decl.prop, value);
            }
          }
        });
      }

      // ── 3. Resolve every remaining color-mix() ──────────────────────────
      const emitted = new Map(); // hash -> { expr, perTheme: Map(theme -> value) }
      root.walkDecls((decl) => {
        if (themeBlockRules.has(decl.parent)) return;
        if (!decl.value.toLowerCase().includes("color-mix(")) return;
        const exprs = extractColorMixExprs(decl.value);
        if (!exprs) {
          fail(decl, decl.value, "unbalanced parentheses");
          return;
        }
        let value = decl.value;
        for (const expr of exprs) {
          const normalized = normalizeExpr(expr);
          let perTheme;
          try {
            perTheme = new Map(
              themeIds.map((theme) => [
                theme,
                formatColor(evalColorMixExpr(expr, theme, decl)),
              ]),
            );
          } catch (e) {
            fail(decl, normalized, e.message);
            continue;
          }
          const values = [...perTheme.values()];
          const allSame = values.every((v) => v === values[0]);
          if (allSame) {
            value = value.replace(expr, values[0]);
          } else {
            let hash = hashExpr(normalized);
            // Extremely unlikely, but guard hash collisions deterministically.
            while (emitted.has(hash) && emitted.get(hash).expr !== normalized) {
              hash = hashExpr(`${normalized}#${hash}`);
            }
            if (!emitted.has(hash))
              emitted.set(hash, { expr: normalized, perTheme });
            value = value.replace(expr, `var(--cmix-${hash})`);
          }
        }
        decl.value = value;
      });

      if (errors.length > 0) {
        throw new Error(
          `[resolve-color-mix] ${errors.length} unresolvable color-mix() expression(s); ` +
            `shipped CSS must not contain color-mix() (Chromium 104 baseline):\n` +
            errors.join("\n"),
        );
      }

      // ── 4. Append per-theme --cmix-* variables to every theme block ─────
      if (emitted.size > 0) {
        for (const theme of themeIds) {
          const rule = themeRules.get(theme);
          if (!rule) continue;
          for (const [hash, entry] of emitted) {
            rule.append({
              prop: `--cmix-${hash}`,
              value: entry.perTheme.get(theme),
            });
          }
        }
      }
    },
  };
}

resolveColorMix.postcss = true;
