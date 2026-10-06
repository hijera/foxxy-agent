---
description: The website under site/ - copy in both languages, baked data, real screenshots, the docs/ overlay contract
paths:
  - "site/**"
  - ".github/workflows/site.yaml"
---

# Website (`site/`)

The site published on GitHub Pages together with `docs/`. Guide: **`docs/contributing/website.md`**.

- **Copy lives in `src/i18n/en.json` and `src/i18n/ru.json` only**, with the same keys, arrays and `{slots}` in both (`test/i18n.test.ts`). No string literals for visible text in TSX. Russian copy follows **`.claude/rules/russian-wording.md`**.
- **No runtime network.** Downloads and change notes are baked by `scripts/build-data.mjs` into `src/generated/` (gitignored). The page must never call `api.github.com`; `scripts/assemble.mjs` fails a bundle that does.
- **The comparison** (`src/data/compare.json`): every harness has a cell in every column, both languages, a verdict from the fixed set, and a source in `sources`. Write "no data" (`unknown`) instead of guessing, and keep `compiled` current when rows change.
- **Screenshots are real captures** of current builds - never mockups, never edited - in `public/screenshots/` as `<surface>-<view>-<theme>-<lang>.webp`, listed with their size in `src/data/demos.json` (`test/demos.test.ts`). They never go under `docs/assets/`.
- **The overlay contract with `docs/`**: the Pages artifact is `docs/` as it is with the build on top. The site may not add a path `docs/` already has, and `docs/` may not add `index.html`, `compare/`, `changelog/`, `site-assets/` or `screenshots/` (`test/assemble.test.ts`). `updatePlugins.xml` and `config.schema.json` must survive every deploy.
- **`site.yaml`**: one job, checks out `main`, publishes `docs/` even when the site build fails, and is called by `tag-on-merge.yaml` after the build jobs and by `plugin-repository.yaml` after a rollback (`editors/intellij/updateplugins/pages_workflow_test.go`).
- Before finishing a change under `site/`: **`make site-check`** green, and screenshots of every changed page in the PR (light and dark, `ru` and `en`, 390 and 1280 px when the layout differs).
