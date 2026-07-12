# Upstream sync tracker

Отслеживает, до какого коммита `upstream/main` (coddy-agent) мы портировали изменения
в этот форк (foxxyCode). Форк полностью ребрендирован, поэтому `git merge upstream/main`
не применяется — коммиты портируются вручную с заменой токенов `coddy → foxxycode`.

- **upstream:** `https://github.com/coddy-project/coddy-agent` (remote `upstream`)
- **Порядок обновления:** `git fetch upstream --prune`, затем сравнить
  `git log --oneline <last-synced>..upstream/main` и портировать непортированное.

---

## Последняя синхронизация

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-07-12 |
| **Синхронизировано до `upstream/main`** | `55cc476` — *Merge pull request #51 from coddy-project/feature/workspace-switcher* (2026-07-12) |
| **Ближайший upstream-тег** | `0.9.34` |
| **Наш коммит-порт** | `f72fd7a` — *last merge from coddy* |

### Что портировано в этой волне
- **Workspace switching** (upstream `9434f34`, `ef821e0`, `ad9b975`, `486a7bf`) — переключение
  папки / git-ветки / worktree из композера: `internal/gitws`, `external/httpserver/workspace_context.go`,
  `WorkspaceChips.tsx` + модалка/хелперы, openapi, BDD godog-сьют.
- **Model-sync fix** (upstream `0ac781f`) — `applyModelsChange.ts` + проводка в `SettingsSection.tsx`.

### Пропущено как уже покрытое / неактуальное
- `284c6a4` (config schema sync rule) — уже есть в `.claude/rules/workflow.md`.
- `e9f7982` (Windows install docs) — в основном уже есть (`refreshenv` в `docs/install.md`).
- `4b77c9d` (python e2e tweak) — незначительно.
- Более ранние `562705b`, `0f3caff`, `71311ff`, `11cc5e9`, `d080678`, `2882e0d`, `cc61d1a`,
  `0b270aa` (NeuralDeep) и др. — покрыты прежними sync-коммитами (`b115a5b`, `3559301`,
  `84c249f`, `5f8c0a5`, `62116a3`).

---

## Как обновить этот файл в следующий раз

1. `git fetch upstream --prune`
2. `git log --oneline --no-merges 55cc476..upstream/main` — список кандидатов.
3. Портировать непортированное (ребренд `coddy → foxxycode`; см. `AGENTS.md` / память форка).
4. Прогнать гейты: `make test`, `make lint`, `npm --prefix external/ui run build:go`.
5. Обновить таблицу «Последняя синхронизация» выше на новый `upstream/main`.
