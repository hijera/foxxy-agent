// @ts-check
// Reads the fork's GitHub Releases and picks the download links the landing page shows. Asset
// names carry the version (foxxycode-vscode-0.2.93.vsix), so there is no stable
// releases/latest/download/<name> address for them: the links are baked at build time instead.

import { compareSemver } from "./changelog.mjs";

/** @typedef {{ name: string, browser_download_url: string, size: number }} Asset */
/** @typedef {{ tag_name: string, html_url: string, published_at: string | null, draft: boolean, prerelease: boolean, assets: Asset[] }} Release */
/** @typedef {{ version: string, url: string, size: number, name: string }} Download */
/** @typedef {"vscode" | "intellij" | "desktop" | "cliWindows"} DownloadKind */

const SEMVER_TAG = /^\d+\.\d+\.\d+$/;

/** @type {Record<DownloadKind, RegExp>} */
export const ASSET_PATTERNS = {
  vscode: /^foxxycode-vscode-(\d+\.\d+\.\d+)\.vsix$/,
  intellij: /^foxxycode-intellij-(\d+\.\d+\.\d+)\.zip$/,
  desktop: /^foxxycode-desktop_(\d+\.\d+\.\d+)_windows_amd64\.zip$/,
  cliWindows: /^foxxycode_(\d+\.\d+\.\d+)_windows_amd64\.zip$/,
};

/**
 * Published SemVer releases, newest first. Drafts, prereleases and tags that are not X.Y.Z are
 * left out.
 * @param {Release[]} releases
 */
export function publishedReleases(releases) {
  return releases
    .filter((r) => !r.draft && !r.prerelease && SEMVER_TAG.test(r.tag_name))
    .sort((a, b) => compareSemver(b.tag_name, a.tag_name));
}

/**
 * For each download, the newest release that actually has that asset: a release whose VS Code job
 * failed still leaves the previous VSIX on the page instead of a dead link.
 * @param {Release[]} releases
 * @returns {Record<DownloadKind, Download | null>}
 */
export function pickDownloads(releases) {
  const sorted = publishedReleases(releases);
  /** @type {Record<DownloadKind, Download | null>} */
  const out = { vscode: null, intellij: null, desktop: null, cliWindows: null };
  for (const kind of /** @type {DownloadKind[]} */ (Object.keys(ASSET_PATTERNS))) {
    for (const release of sorted) {
      const asset = release.assets.find((a) => ASSET_PATTERNS[kind].test(a.name));
      if (asset) {
        out[kind] = {
          version: release.tag_name,
          url: asset.browser_download_url,
          size: asset.size,
          name: asset.name,
        };
        break;
      }
    }
  }
  return out;
}

/**
 * Fetches up to 50 releases of `repo` ("owner/name"), retrying transient failures.
 * @param {string} repo
 * @param {{ token?: string | undefined, fetchImpl?: typeof fetch, attempts?: number }} [options]
 * @returns {Promise<Release[]>}
 */
export async function fetchReleases(repo, { token, fetchImpl = fetch, attempts = 3 } = {}) {
  /** @type {Record<string, string>} */
  const headers = { Accept: "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28" };
  if (token) headers.Authorization = `Bearer ${token}`;
  let lastError = /** @type {unknown} */ (null);
  for (let attempt = 1; attempt <= attempts; attempt++) {
    try {
      const res = await fetchImpl(`https://api.github.com/repos/${repo}/releases?per_page=50`, { headers });
      if (!res.ok) throw new Error(`GitHub API answered ${res.status} ${res.statusText}`);
      return /** @type {Release[]} */ (await res.json());
    } catch (err) {
      lastError = err;
      if (attempt < attempts) await new Promise((r) => setTimeout(r, 1000 * attempt));
    }
  }
  throw lastError;
}
