# The website

How the FoxxyCode website at [hijera.github.io/foxxy-agent](https://hijera.github.io/foxxy-agent/) is built and published: the source under `site/`, the data it bakes at build time, the English changelog twins it reads, the comparison it carries, the screenshots, and the workflow that puts it on GitHub Pages together with this `docs/` tree.

## What it is

Three pages, each in English and Russian and in a light and a dark theme:

| Page | What it shows |
|------|---------------|
| `/` | What FoxxyCode is and adds over coddy-agent, the latest download of each package (VS Code, JetBrains, the Windows desktop app, the Windows console), screenshots of the plugins, the desktop app and the web UI, questions |
| `/compare/` | FoxxyCode, Coddy and the other agent harnesses side by side in six tables, IDE and desktop first, with sources |
| `/changelog/` | The last ten releases of the two plugins, merged per version, in both languages |

The Settings footer of the web UI links to the site next to "API docs", so every surface - both editor plugins, the desktop app and the browser - reaches it.

## Layout

```text
site/
  index.html, compare/index.html, changelog/index.html   entry pages (Vite multi-page build)
  scripts/build-data.mjs      bakes src/generated/ before a build
  scripts/assemble.mjs        lays the build over docs/ into the Pages artifact
  scripts/lib/                changelog parser, releases picker, head boot script
  src/i18n/en.json, ru.json   every string of the pages
  src/data/compare.json       the comparison tables
  src/data/demos.json         the screenshots and their sizes
  public/screenshots/         the screenshots (WebP)
  test/                       vitest suites
```

React and Vite, no router and no i18n library. The language is `?lang=ru|en`, then the visitor's stored choice, then the browser languages; the theme is the stored choice, then `prefers-color-scheme`. A script injected into `<head>` (`scripts/lib/boot.mjs`) settles both on `<html>` before the stylesheet loads, so the first paint already has the right colours and language.

```bash
cd site
npm ci
npm run data:offline   # or `npm run data` for the live releases
npm run dev            # http://localhost:5175/foxxy-agent/
npm test
```

`make site-check` from the repository root runs what CI runs: the offline bake in strict mode, the type-check, the tests, the production build and the assembly.

## Baked data

The page never calls the GitHub API. `scripts/build-data.mjs` writes `src/generated/` (gitignored) before every build:

- **`release.json`**: for each download card, the newest published release that has that asset - `foxxycode-vscode-<v>.vsix`, `foxxycode-intellij-<v>.zip`, `foxxycode-desktop_<v>_windows_amd64.zip`, `foxxycode_<v>_windows_amd64.zip` - with its size. Asset names carry the version, so there is no stable download address to link instead. When the API cannot be read, the cards link to the releases page. `GITHUB_TOKEN` is used when set.
- **`changelog.json`**: the two plugin changelogs and their English twins, merged per version. An entry both plugins carry word for word is shown once with both badges. An `## Unreleased` section gets the latest release's number only when the changelog at that tag carries the same section; otherwise it is left out until it ships.

`--offline <releases.json>` reads the releases from a file instead of the API, and `--strict` fails when a shown entry has no English text. CI uses both.

## The English changelog twins

The plugins show their change notes in Russian (`editors/intellij/CHANGELOG.md`, `editors/vscode/CHANGELOG.md`). The website shows them in both languages, so each file has an English twin next to it, `CHANGELOG.en.md`, with the same headings from the top down and the same entries. The rule is in `.claude/rules/release-changelog.md` §4; `editors/intellij/changelog_twin_test.go` and `editors/vscode/test/changelog.test.ts` fail when a twin drifts.

## The comparison

`src/data/compare.json` holds the harnesses (id, name, link, role) and six tables. Every cell has a verdict - `yes`, `partial`, `no`, `unknown`, `na`, or `info` for plain facts such as a language - and either one text for both languages (`t`) or `en` and `ru`. `test/compare.test.ts` requires a cell for every harness in every column, both languages, FoxxyCode first and Coddy marked as upstream.

The rows follow each project's own documentation, starting from [coddy.dev/compare](https://coddy.dev/compare/) for the harnesses both comparisons share. "No data" is the honest answer where a project does not document something. When a row is updated, update `compiled` and the sources list, and the copy in `src/i18n/*.json` that summarises the comparison.

## Screenshots

`public/screenshots/` holds real captures of the current builds, never mockups: IntelliJ from the UI-test sandbox (the intellij-plugin-uitest skill), VS Code from an extension development host, the desktop window (WebView2, captured with its frame) and the web UI in headless Chrome. They were taken against a scripted OpenAI-compatible stub model that plays one turn on a small Go project, so no key is needed and every language and theme shows the same turn. File names are `<surface>-<view>-<theme>-<lang>.webp`; `src/data/demos.json` lists each with its size, and `test/demos.test.ts` checks the files, their dimensions and that every surface has both languages. A missing theme falls back to the same language in the other theme.

These files belong to the site, not to the documentation: they never go under `docs/assets/`, whose every file must be referenced by a page.

## Publishing

`.github/workflows/site.yaml` (**Website**) builds the site and publishes GitHub Pages. The artifact is the whole `docs/` tree as it is on `main`, with the build laid over it by `scripts/assemble.mjs`. Every address `docs/` already had stays valid - most importantly `updatePlugins.xml`, which JetBrains IDEs poll for plugin updates, and `config.schema.json`, which FoxxyCode writes into every config it saves. The assembly fails instead of overwriting a documentation file, so the site may never add a path that `docs/` already has (`index.html`, `compare/`, `changelog/`, `site-assets/`, `screenshots/`). If the site itself fails to build, `docs/` is published alone and the run still fails.

The workflow runs:

- from **Tag release on merge**, after the binaries, the IntelliJ plugin and the VS Code extension, even when one of them failed. The IntelliJ job commits the new `updatePlugins.xml` to `main` with `GITHUB_TOKEN`, and such a push starts no workflow, so this call is the only deploy it gets;
- from **IntelliJ plugin repository**, after a manual rollback or re-sync;
- by hand, **Actions → Website → Run workflow**, after a manually pushed tag or a change that did not go through a pull request.

It checks out `main`, not the commit that started the run, so it always sees the document the release just committed. `editors/intellij/updateplugins/pages_workflow_test.go` pins these calls and the shape of the job.

Pages must use **GitHub Actions** as its source. While the repository still deploys `main:/docs` from the branch, the workflow builds and uploads the artifact but skips the deploy with a notice. The switch and the rollback are described in [Release setup](release-setup.md) §2.

## Checks after a deploy

```bash
gh api repos/hijera/foxxy-agent/pages --jq .build_type                          # workflow
curl -fsS https://hijera.github.io/foxxy-agent/updatePlugins.xml | grep version=
curl -fsS https://hijera.github.io/foxxy-agent/config.schema.json | cmp - internal/config/config.schema.json
curl -fsSI https://hijera.github.io/foxxy-agent/compare/
```
