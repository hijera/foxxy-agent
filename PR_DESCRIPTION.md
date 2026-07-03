# Redesign the Settings UI into a tabbed master–detail interface

## Summary

The Settings drawer used to render the entire configuration as a single long,
scrolling sheet, with Appearance and Skills bolted on as separate flyout panels.
This PR turns it into a **tabbed master–detail** experience that is schema-driven,
responsive, and lets each configuration section be saved independently. It also
adds a backend endpoint to fetch a provider's available models (Kilo-style) and
replaces every `<select>` with an editable combobox.

## Motivation

- The single-sheet form was hard to navigate and edit.
- Lists (providers, models, MCP servers) were all expanded at once with no way to
  focus a single item.
- There was no way to discover a provider's available models — model ids had to be
  typed by hand.
- Saving always wrote the whole config; users wanted to save one section at a time.

## Changes

### Backend — fetch a provider's model list

- `internal/llm/model_list.go`: new `ListModels(ctx, ProviderInput)` returning
  `[]ModelEntry`. It queries `{base}/models` (OpenAI / OpenAI-compatible, `Bearer`
  auth) or `{base}/v1/models` (Anthropic, `x-api-key` + `anthropic-version`),
  reuses the existing per-provider proxy HTTP client, applies a 15s timeout, and
  de-duplicates and sorts results. Non-2xx responses return a typed error.
- `external/httpserver/providers_models_http.go`: new `GET /coddy/providers/{name}/models`.
  The provider is resolved from the **saved** config, so its credentials
  (`api_key` / `api_key_command` / `NAME_API_KEY` env) and `proxy` apply
  server-side without exposing secrets. Returns `{ok:true, models:[{id,name}]}` on
  success, `{ok:false, error, models:[]}` (HTTP 200) when the upstream call fails
  so the UI degrades to manual entry, and `404` for an unknown provider.
- Registered in `server.go`; spec added to `openapi.go`; documented in
  `docs/http-api.md`.

### Frontend — tabbed settings shell

- Tabs are derived from the live config JSON Schema (`settingsSections.ts`): each
  top-level property becomes a tab (label from its `title`); the rarely edited tail
  (`scheduler`, `prompts`, `instructions`, `logger`, `sessions`, `gateways`) is
  folded into a single **System** tab; a client-side **Appearance** tab is appended.
- `Settings.tsx` is refactored into a `SettingsNav` (section list) + `SettingsSection`
  (active panel) layout while keeping the existing load / validate / save flow.
- **List sections** (LLM Providers, Logical Models, MCP Servers) use a master–detail
  pattern (`SettingsArraySection.tsx`): a list of named buttons with Add / Remove;
  Add or selecting a row hides the list and shows the item form with "← Back to list".
- `SchemaForm.tsx` gains a small, backward-compatible `fieldOverride` hook so custom
  editors can be injected without forking the generic renderer.
- **Model editors**:
  - `ModelField.tsx` (Logical Models): pick a provider, "Fetch models" pulls the
    provider's advertised models into a combobox, with manual entry as fallback.
  - `ModelPicker.tsx` (ReAct Agent / Memory default model): pick from the configured
    logical models or type one manually.
- **Skills** tab (`SkillsSection.tsx`) combines the schema-driven `skills.dirs`
  editor with the installed-skills list (enable/disable), folding in the former
  Skills flyout (`SkillsPanel.tsx` removed).
- **Appearance** becomes a tab; the theme swatch grid is extracted into a reusable
  `AppearanceThemePicker`. The Appearance/Skills flyout dock-cluster is retired and
  the related state/props are removed from `App.tsx`.

### UX polish

- **Editable comboboxes** (`Combobox.tsx`) replace every `<select>` in settings
  (schema enums, provider, model fields): focus or the caret shows all options,
  typing filters them, and any typed value is kept (free text).
- **Thin scrollbars** throughout the settings drawer (cross-browser).
- On narrow shells the **horizontal tab strip hides its scrollbar** and shows
  clickable **edge arrows** that page through the tabs (disabled at the ends).
- **Button hover highlights**, a **refresh** dissolve/reappear animation with a
  spinning icon, and a **save** success pulse. All motion respects
  `prefers-reduced-motion`.
- **Per-section Save**: each section has its own "Save section" button. It overlays
  only that section's values onto the latest on-disk config before validating and
  writing, so saving one section does not commit unsaved edits made in others.
  A footer "Save all sections" and "Reload" remain.

### Responsive layout

- Desktop (`min-width: 1200px`): wider drawer with a vertical section rail on the
  left and the content panel on the right.
- Mobile (`max-width: 1199px`): a horizontal, scrollbar-free tab strip with edge
  arrows on top and the content stacked below.

## API

| Method | Path | Description |
| ------ | ---- | ----------- |
| GET | `/coddy/providers/{name}/models` | Fetch a saved provider's advertised models. `200 {ok:true, models:[{id,name}]}` on success; `200 {ok:false, error, models:[]}` on upstream failure; `404` for an unknown provider. |

No breaking changes to existing endpoints. `PUT /coddy/config` and per-section saves
both send a complete, valid config document (non-UI keys such as `httpserver` are
preserved).

## Testing

- Go: new table-driven tests for `llm.ListModels` (OpenAI/Anthropic happy paths,
  non-2xx, non-JSON, unsupported type) and for the new HTTP endpoint (happy path,
  unknown provider 404, graceful failure). Verified across the relevant build-tag
  combinations (`http`, `http,ui`, `http,scheduler,ui,memory`).
- UI: new Vitest suites for `Combobox`, `ModelField`, `ModelPicker`,
  `SettingsArraySection`, and `settingsSections`; existing suites updated. Full UI
  test suite green.
- Manual verification in a browser at desktop and mobile widths: tab navigation,
  master-detail add/edit/remove, live provider model fetch + manual fallback,
  combobox behavior (zero `<select>` left in the drawer), per-section save (merged
  body inspected), thin/hidden scrollbars, edge arrows, and the hover/refresh/save
  animations.

## Build / notes

- The SPA is embedded via `go:embed`, so rebuild the assets before building the
  binary:
  ```
  cd external/ui && npm run build:go   # vite build + sync to the embed location
  cd ../.. && go build -tags "http ui" -o build/coddy.exe ./cmd/coddy/
  ```
- In Firefox, `scrollbar-width` only supports `auto | thin | none`, so the thin
  scrollbars are as thin as the browser allows (custom pixel widths apply to
  Chromium/WebKit only).

## Files of note

- Backend: `internal/llm/model_list.go`, `external/httpserver/providers_models_http.go`,
  `external/httpserver/server.go`, `external/httpserver/openapi.go`, `docs/http-api.md`.
- Frontend (`external/ui/src/ui/settings/`): `Settings.tsx`, `SettingsNav.tsx`,
  `SettingsSection.tsx`, `SettingsArraySection.tsx`, `Combobox.tsx`, `ModelField.tsx`,
  `ModelPicker.tsx`, `SkillsSection.tsx`, `SchemaForm.tsx`, `settingsSections.ts`,
  `useProviderModels.ts`; plus `theme/AppearanceModal.tsx`, `App.tsx`, `styles.css`.
- Docs: `DESIGN.md`.
