// Typed views of the data baked by scripts/build-data.mjs and of the static links.
import releaseJson from "../generated/release.json";
import changelogJson from "../generated/changelog.json";
import type { Download, DownloadKind } from "../../scripts/lib/releases.mjs";
import type { MergedVersion } from "../../scripts/lib/changelog.mjs";

export type { Inline, Block, Surface, MergedVersion, MergedEntry } from "../../scripts/lib/changelog.mjs";
export type { DownloadKind } from "../../scripts/lib/releases.mjs";

export interface ReleaseData {
  repo: string;
  repoUrl: string;
  releasesUrl: string;
  intellijRepositoryUrl: string;
  apiOk: boolean;
  latest: { version: string; url: string; date: string | null } | null;
  downloads: Record<DownloadKind, Download | null>;
}

export interface ChangelogData {
  releasesUrl: string;
  versions: MergedVersion[];
}

export const release = releaseJson as unknown as ReleaseData;
export const changelog = changelogJson as unknown as ChangelogData;

const BASE = import.meta.env.BASE_URL;

export const pages = {
  home: BASE,
  compare: `${BASE}compare/`,
  changelog: `${BASE}changelog/`,
};

export const links = {
  repo: release.repoUrl,
  releases: release.releasesUrl,
  issues: `${release.repoUrl}/issues`,
  docs: `${release.repoUrl}/tree/main/docs`,
  forkDoc: `${release.repoUrl}/blob/main/docs/getting-started/foxxycode-and-coddy.md`,
  coddySite: "https://coddy.dev",
  coddyRepo: "https://github.com/coddy-project/coddy-agent",
  license: `${release.repoUrl}/blob/main/LICENSE`,
};

/** A repository path (docs/…, editors/…) as a link GitHub renders. */
export function repoFile(path: string): string {
  return `${release.repoUrl}/blob/main/${path}`;
}

/** Fills `{name}` slots of a message. */
export function format(message: string, values: Record<string, string | number>): string {
  return message.replace(/\{(\w+)\}/g, (slot, name: string) => (name in values ? String(values[name]) : slot));
}

export function formatSize(bytes: number, lang: "en" | "ru"): string {
  const mb = bytes / (1024 * 1024);
  return `${mb.toLocaleString(lang === "ru" ? "ru-RU" : "en-US", { maximumFractionDigits: 0 })} ${lang === "ru" ? "МБ" : "MB"}`;
}
