# FoxxyCode website

The site published at <https://hijera.github.io/foxxy-agent/>: the landing page, the comparison
and the plugin changelog, in English and Russian, in a light and a dark theme. React and Vite,
no router and no i18n library.

```sh
npm ci
npm run data:offline   # or `npm run data` for the live GitHub Releases (GITHUB_TOKEN is used when set)
npm run dev            # http://localhost:5175/foxxy-agent/
npm test
```

- **Copy** lives in `src/i18n/en.json` and `src/i18n/ru.json`; both must have the same keys.
- **Comparison data** lives in `src/data/compare.json`.
- **Screenshot list** lives in `src/data/demos.json`, and the files in `public/screenshots/`.
- **Build data.** `scripts/build-data.mjs` bakes the downloads and the recent change notes into
  `src/generated/` before a build. The page never calls the GitHub API.
- **Publishing.** `scripts/assemble.mjs` lays the build over a copy of `docs/`; that artifact is
  what GitHub Pages serves.

`make site-check` from the repository root is what CI runs.
