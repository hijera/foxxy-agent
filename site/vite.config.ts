/// <reference types="vitest/config" />

import path from "node:path";
import { defineConfig, searchForWorkspaceRoot, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import { BOOT_SCRIPT } from "./scripts/lib/boot.mjs";

const here = import.meta.dirname;

// Settles language and theme before the first paint, in every page (see scripts/lib/boot.mjs).
const headBoot: Plugin = {
  name: "foxxy-site-head-boot",
  transformIndexHtml() {
    return [{ tag: "script", children: BOOT_SCRIPT, injectTo: "head-prepend" }];
  },
};

export default defineConfig({
  // GitHub Pages serves the project at /<repository>/; site.yaml passes it in.
  base: process.env.SITE_BASE ?? "/foxxy-agent/",
  plugins: [react(), headBoot],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // Not "assets": the published artifact also carries docs/assets/ from the documentation.
    assetsDir: "site-assets",
    rollupOptions: {
      input: {
        index: path.resolve(here, "index.html"),
        compare: path.resolve(here, "compare/index.html"),
        changelog: path.resolve(here, "changelog/index.html"),
      },
    },
  },
  server: {
    port: 5175,
    strictPort: true,
    // Logos and screenshots are imported straight from docs/assets.
    fs: { allow: [searchForWorkspaceRoot(here), path.resolve(here, "../docs/assets")] },
  },
  test: {
    environment: "jsdom",
    include: ["test/**/*.test.{ts,tsx}"],
  },
});
