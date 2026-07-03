/// <reference types="vitest/config" />

import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
// @ts-ignore -- plain .mjs module without type declarations
import resolveColorMix from "./postcss-resolve-color-mix.mjs";

const backend = (process.env.CODDY_UI_BACKEND || "").trim();

export default defineConfig({
  root: "src",
  publicDir: path.resolve(import.meta.dirname, "public"),
  plugins: [react()],
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
            "/coddy": backend,
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
