// @ts-check
// Bakes the data the site renders into src/generated/: the latest downloads from the GitHub
// Releases API (release.json) and the recent change notes of both plugins in Russian and English
// (changelog.json). The page never calls the API itself.
//
//   node scripts/build-data.mjs                         online: GITHUB_TOKEN is used when set
//   node scripts/build-data.mjs --offline <releases.json> --strict
//
// --offline reads releases from a file (CI and tests) and skips the git lookup that decides which
// release shipped the `## Unreleased` section. --strict fails when a shown entry has no English
// translation or the API cannot be read; without it the build falls back and warns, so a release
// is never held up by the website.

import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  mergeChangelogs,
  parseChangelog,
  resolveUnreleased,
  sameUnreleased,
  selectVersions,
  translationFloor,
  compareSemver,
} from "./lib/changelog.mjs";
import { fetchReleases, pickDownloads, publishedReleases } from "./lib/releases.mjs";
import { fileAtTag } from "./lib/git.mjs";

const siteRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = resolve(siteRoot, "..");
const VERSIONS_SHOWN = 10;

/** @param {string[]} argv */
function parseArgs(argv) {
  /** @type {{ offline: string | null, strict: boolean, out: string }} */
  const args = { offline: null, strict: false, out: join(siteRoot, "src", "generated") };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--offline") args.offline = resolve(siteRoot, argv[++i] ?? "");
    else if (arg === "--strict") args.strict = true;
    else if (arg === "--out") args.out = resolve(argv[++i] ?? "");
    else throw new Error(`unknown argument ${arg}`);
  }
  return args;
}

/** @param {string} repo */
export function repoLinks(repo) {
  const [owner = "", name = ""] = repo.split("/");
  return {
    repo,
    repoUrl: `https://github.com/${repo}`,
    releasesUrl: `https://github.com/${repo}/releases`,
    intellijRepositoryUrl: `https://${owner.toLowerCase()}.github.io/${name}/updatePlugins.xml`,
  };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const repo = process.env.SITE_REPO || "hijera/foxxy-agent";
  const links = repoLinks(repo);

  /** @type {import("./lib/releases.mjs").Release[]} */
  let releases = [];
  let apiOk = true;
  try {
    releases = args.offline
      ? JSON.parse(readFileSync(args.offline, "utf8"))
      : await fetchReleases(repo, { token: process.env.GITHUB_TOKEN });
  } catch (err) {
    apiOk = false;
    const message = `could not read the releases of ${repo}: ${err instanceof Error ? err.message : String(err)}`;
    if (args.strict) throw new Error(message);
    console.warn(`build-data: ${message}; the download cards will link to the releases page`);
  }

  const published = publishedReleases(releases);
  const latest = published[0] ?? null;
  const release = {
    ...links,
    apiOk,
    latest: latest ? { version: latest.tag_name, url: latest.html_url, date: latest.published_at } : null,
    downloads: pickDownloads(releases),
  };

  const surfaces = /** @type {const} */ ({
    intellij: "editors/intellij",
    vscode: "editors/vscode",
  });
  /** @type {Record<"intellij" | "vscode", { ru: import("./lib/changelog.mjs").Section[], en: import("./lib/changelog.mjs").Section[] }>} */
  const sources = { intellij: { ru: [], en: [] }, vscode: { ru: [], en: [] } };
  /** @type {string[]} */
  const floors = [];
  for (const [surface, dir] of /** @type {["intellij" | "vscode", string][]} */ (Object.entries(surfaces))) {
    const ruPath = `${dir}/CHANGELOG.md`;
    const enPath = `${dir}/CHANGELOG.en.md`;
    const ru = parseChangelog(readFileSync(join(repoRoot, ruPath), "utf8"));
    const en = parseChangelog(readFileSync(join(repoRoot, enPath), "utf8"));
    let unreleasedVersion = null;
    if (latest) {
      if (args.offline) {
        unreleasedVersion = latest.tag_name;
      } else {
        const atTag = fileAtTag(repoRoot, latest.tag_name, ruPath);
        if (atTag && sameUnreleased(ru, parseChangelog(atTag))) unreleasedVersion = latest.tag_name;
      }
    }
    sources[surface] = {
      ru: resolveUnreleased(ru, unreleasedVersion),
      en: resolveUnreleased(en, unreleasedVersion),
    };
    const floor = translationFloor(sources[surface].en);
    if (floor) floors.push(floor);
  }

  const floor = floors.length > 0 ? floors.reduce((max, v) => (compareSemver(v, max) > 0 ? v : max)) : null;
  const versions = selectVersions(mergeChangelogs(sources), { limit: VERSIONS_SHOWN, floor });
  const untranslated = versions.filter((v) => !v.translated).map((v) => v.version);
  if (untranslated.length > 0) {
    const message = `no English translation for ${untranslated.join(", ")} in CHANGELOG.en.md`;
    if (args.strict) throw new Error(message);
    console.warn(`build-data: ${message}; the English page shows the Russian text for them`);
  }

  mkdirSync(args.out, { recursive: true });
  writeFileSync(join(args.out, "release.json"), `${JSON.stringify(release, null, 2)}\n`);
  writeFileSync(
    join(args.out, "changelog.json"),
    `${JSON.stringify({ releasesUrl: links.releasesUrl, versions }, null, 2)}\n`,
  );
  console.log(
    `build-data: ${latest ? `latest ${latest.tag_name}` : "no releases"}, ` +
      `${versions.length} changelog versions (${versions[0]?.version ?? "-"} … ${versions.at(-1)?.version ?? "-"})`,
  );
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((err) => {
    console.error(`build-data: ${err instanceof Error ? err.message : String(err)}`);
    process.exit(1);
  });
}
