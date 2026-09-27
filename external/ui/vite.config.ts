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
    // (JCEF baseline, see docs/contributing/intellij-embedding.md). Applies in dev too.
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
    // plugin embedding (docs/contributing/intellij-embedding.md). Do not raise without
    // bumping the minimum supported IDE.
    target: "chrome104",
    cssTarget: "chrome104",
    // Lightning CSS (Vite 8 default) can drop unprefixed `backdrop-filter` when `-webkit-backdrop-filter`
    // is present, which breaks blur in Firefox (and some stacks). Esbuild preserves both declarations.
    cssMinify: "esbuild",
    // Every asset the bundle imports must be inlined, whatever its size.
    // scripts-sync-to-go.mjs ships index.html, styles.css and app.js and
    // nothing else, so an asset Vite emits as its own file is missing from the
    // go:embed set and 404s at runtime. That is not hypothetical: when the
    // brand mark became a traced fox the wordmarks grew past the 4 KB default
    // and the sign-in screen - the one page a browser sees before it has any
    // credential, so it cannot fetch anything anyway - rendered a broken image.
    // The sync script fails the build on an unexpected file in dist/ as well.
    assetsInlineLimit: () => true,
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
  // The SharedWorker that holds GET /foxxycode/events for every tab
  // (src/ui/chat/eventsWorker.ts). It is its own script by necessity, so it gets a
  // fixed name next to app.js for go:embed. IIFE, not ES: the file then has no
  // import or export and runs whether a browser honours `type: "module"` or not.
  worker: {
    format: "iife",
    rollupOptions: {
      output: {
        entryFileNames: "events-worker.js",
      },
    },
  },
} as any);
