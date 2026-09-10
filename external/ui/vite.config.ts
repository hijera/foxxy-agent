/// <reference types="vitest/config" />

import path from "node:path";
import { readFileSync, readdirSync } from "node:fs";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
// @ts-ignore -- plain .mjs module without type declarations
import resolveColorMix from "./postcss-resolve-color-mix.mjs";

const backend = (process.env.FOXXYCODE_UI_BACKEND || "").trim();
const cacheDir = (process.env.FOXXYCODE_UI_VITE_CACHE_DIR || "").trim();

// Retain vendored grammar notices in the embedded JS distribution. The minifier
// otherwise strips source comments, including comments marked as licenses.
const grammarDirectory = path.resolve(import.meta.dirname, "src/ui/markdown/grammars");
const syntaxLicenseBanner = readdirSync(grammarDirectory)
  .filter((name) => name.endsWith(".LICENSE"))
  .sort()
  .map((name) => `/* ${name}\n${readFileSync(path.join(grammarDirectory, name), "utf8")}\n*/`)
  .join("\n");

const syntaxLicensePlugin: Plugin = {
  name: "syntax-grammar-notices",
  enforce: "post",
  generateBundle(_options, bundle) {
    for (const output of Object.values(bundle)) {
      if (output.type === "chunk") output.code += `\n${syntaxLicenseBanner}\n`;
    }
  },
};

export default defineConfig({
  root: "src",
  publicDir: path.resolve(import.meta.dirname, "public"),
  ...(cacheDir ? { cacheDir } : {}),
  plugins: [react(), syntaxLicensePlugin],
  css: {
    // Resolves color-mix() to Chromium-104-safe literals at build time
    // (JCEF baseline, see docs/intellij-embedding.md). Applies in dev too.
    postcss: {
      plugins: [resolveColorMix()],
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
  },
  server: {
    port: 5173,
    strictPort: true,
    ...(backend
      ? {
          proxy: {
            "/v1": backend,
            "/foxxycode": backend,
            "/docs": backend,
            "/openapi.yaml": backend,
            "/openapi.json": backend,
          },
        }
      : {}),
  },
  build: {
    // Chromium 104 is the JCEF baseline for the IntelliJ/PhpStorm 2022.3.3
    // plugin embedding (docs/intellij-embedding.md). Do not raise without
    // bumping the minimum supported IDE.
    target: "chrome104",
    cssTarget: "chrome104",
    // Lightning CSS (Vite 8 default) can drop unprefixed `backdrop-filter` when `-webkit-backdrop-filter`
    // is present, which breaks blur in Firefox (and some stacks). Esbuild preserves both declarations.
    cssMinify: "esbuild",
    outDir: "../dist",
    emptyOutDir: true,
    sourcemap: true,
    cssCodeSplit: false,
    rollupOptions: {
      output: {
        entryFileNames: "app.js",
        assetFileNames: (assetInfo: { name?: string | undefined }) => {
          if (assetInfo.name === "style.css") {
            return "styles.css";
          }
          return "[name][extname]";
        },
        chunkFileNames: "app.js",
        inlineDynamicImports: true,
      },
    },
  },
} as any);
