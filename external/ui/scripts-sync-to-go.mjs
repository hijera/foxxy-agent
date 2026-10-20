import { cp, mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import path from "node:path";

const uiRoot = path.resolve(import.meta.dirname);
const dist = path.join(uiRoot, "dist");

await mkdir(uiRoot, { recursive: true });

const indexSrc = path.join(dist, "index.html");
const html = await readFile(indexSrc, "utf8");

const patched = html
  .replaceAll(/href="\.\/?styles\.css"/g, 'href="/styles.css"')
  .replaceAll(/src="\.\/?app\.js"/g, 'src="/app.js"')
  .replaceAll(/href="\/styles\.css"/g, 'href="/styles.css"')
  .replaceAll(/src="\/app\.js"/g, 'src="/app.js"');

await writeFile(path.join(uiRoot, "index.html"), patched);
await cp(path.join(dist, "styles.css"), path.join(uiRoot, "styles.css"));
await cp(path.join(dist, "app.js"), path.join(uiRoot, "app.js"));
await cp(
  path.join(dist, "events-worker.js"),
  path.join(uiRoot, "events-worker.js"),
);

const docsAssets = path.join(uiRoot, "..", "..", "docs", "assets");
const faviconFiles = [
  ["foxxycode-favicon.svg", "foxxycode-favicon.svg"],
  ["favicon-32.png", "favicon-32.png"],
  ["favicon.ico", "favicon.ico"],
  ["apple-touch-icon.png", "apple-touch-icon.png"],
];
for (const [srcName, destName] of faviconFiles) {
  await cp(path.join(docsAssets, srcName), path.join(uiRoot, destName));
}

// Only the files above reach the go:embed set, so anything else Vite emits into
// dist/ ships nowhere and 404s at runtime. That happened once already: the brand
// wordmarks grew past the default 4 KB inline limit, Vite wrote them out as
// their own files, and the sign-in screen rendered a broken image. vite.config.ts
// now inlines every asset; this is the check that says so out loud if that
// changes again.
const shipped = new Set([
  "index.html",
  "styles.css",
  "app.js",
  "app.js.map",
  // The SharedWorker that shares GET /foxxycode/events between tabs (copied above).
  "events-worker.js",
  "events-worker.js.map",
  ...(await readdir(path.join(uiRoot, "public"))),
]);
const orphans = (await readdir(dist)).filter((name) => !shipped.has(name));
if (orphans.length > 0) {
  console.error(
    `sync-to-go: dist/ holds ${orphans.length} file(s) nothing ships: ${orphans.join(", ")}.\n` +
      "They would 404 at runtime. Inline them (build.assetsInlineLimit in vite.config.ts)\n" +
      "or add them to this script and to external/ui/embed.go.",
  );
  process.exit(1);
}
