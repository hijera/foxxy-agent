# Upstream sync tracker

Отслеживает, до какого коммита `upstream/main` (coddy-agent) мы портировали изменения
в этот форк (foxxyCode). Форк полностью ребрендирован, поэтому `git merge upstream/main`
не применяется — коммиты портируются вручную с заменой токенов `coddy → foxxycode`.

- **upstream:** `https://github.com/coddy-project/coddy-agent` (remote `upstream`)
- **Порядок обновления:** `git fetch upstream --prune`, затем сравнить
  `git log --oneline <last-synced>..upstream/main` и портировать непортированное.

---

## Волна `bc1afb9 → 6666606` (тег `0.9.43`) — ГОТОВО

Крупная волна (42 не-merge коммита, 158 файлов), портирована тремя бандлами отдельными коммитами
на ветке `sync/upstream-6666606`: `f0a2506` (Волна 1), `60af986`+`305fc5a` (Волна 2),
`3b3e812`+`0e75aa7` (Волна 3). Все гейты зелёные (go: default/http/http,memory/memory/scheduler/
http,scheduler; UI: 680 vitest + build:go). Итоги:

- **Волна 1 — Compaction (coddy по умолчанию + тумблер на OpenCode) + чистые фиксы — ГОТОВО.**
  - Слиты два движка компакции в один `compaction:`-блок с полем `engine: coddy | opencode`
    (default `coddy`). coddy — новый движок (`internal/agent/compact.go`,
    `internal/session/compaction.go`, ручная `/compact`, авто-компакция, HTTP
    `POST /foxxycode/sessions/{id}/compact`, UI `CompactionMessage.tsx`); opencode — прежний
    движок форка (`internal/agent/compaction.go`, флаг `Compacted`). Диспетчеризация по движку
    в `internal/agent/react.go` (построение окна истории + триггер + перехват `/compact`).
    Конфиг: `internal/config/compaction.go` (поля `engine`, `keep_recent_turns` вместо
    `keep_last_turns`), jsondto/ui_schema/docs/example + RU-оверлей + фикстура ui-schema.
  - Windows session-fix (upstream `4f57540`): `pathMutex` на `FileStore`, `renameWithRetry`,
    `rename_windows.go`/`rename_other.go`. Чинит флейк `TestConcurrentPatchSessionMetaActivitySync`.
  - fs line-endings (upstream `f6cf51c` + `9111fa8`): новый `internal/tools/fs/line_endings.go`,
    правки `edit.go`/`patch.go`/`patch_v4a.go` **вручную поверх cp1251-слоя** `decodeText`/`encodeText`;
    BDD `features/edit_line_endings.feature`. `/compact` объявляется в слэш-меню
    (`skills.BuiltinCommands`, ACP + HTTP `/foxxycode/slash-commands`).
  - Мелочь: staticcheck-гарды (`99259a7`), `.gitignore *.bak` (`87d1040`). `69ce66c`
    (light-theme кнопка) уже был в форке.
  - Гейты зелёные: default / `http` / `http,memory` / `memory` / `scheduler`, `build:go`.
- **Волна 2 — Remote control / http-auth / env-selector — ГОТОВО (backend `60af986` + UI).**
  - Config: `internal/config/http.go` (+`auth_token`/`public_docs`/`allow_insecure`/`cors`/`remotes`
    + helpers `CORSAllowOrigin`/`EffectiveAuthTokens`), `ui.enabled` влит в форковый `UIConfig`;
    jsondto (редакция токена + `ParseConfigJSONPreservingSecrets`); docs schema/reference/example
    + RU-оверлей + фикстура.
  - HTTP: `external/httpserver/auth.go` (bearer-gate, realm `foxxycode`, SSE `?access_token=`,
    **IDE-роуты `/foxxycode/ide/*` освобождены** от auth), `cors.go` (`X-FoxxyCode-Session-ID`),
    `Handler()` = `corsMiddleware(authGate(mux))`, `--auth-token`/`FOXXYCODE_HTTP_TOKEN` +
    non-loopback-warning в `StartHTTP`, `ui.enabled`-гейт SPA-root, openapi `bearerAuth`.
    Тесты: 13 auth/CORS + IDE-exemption unit. Docs: `docs/remote-control.md`, `docs/http-api.md`.
  - UI env-selector: `env/remoteEnv.ts` (fetch-shim, per-env storage), `env/activeHealth.ts`,
    `env/remoteErrors.ts`, `env/EnvHealthBanner.tsx`, `chat/EnvironmentChip.tsx` (чип в
    composer-workspace-строке, меню Local/remotes/Add, health-точки). Shim ставится в `main.tsx`
    до рендера; `workspaceRecents.ts` неймспейсится по env; чип виден и без workspace-контекста.
    Проверено в браузере (чип «Local», меню открывается, 0 console-ошибок; 671 UI-тест зелёный).
  - ~~Осталось в Волне 2: BDD remote-API parity (`46445df`/`328bc25`)~~ — сделано в `f2f4682`
    (`features/remote_api.feature` + харнесс).
- **Волна 3 — Skills marketplace + plugin command — БЭКЕНД ГОТОВ (коммит следующий), UI TODO.**
  - Config: `skills.go` (+`sources`, +`auto_discovery` + флаг `-skills-auto-discovery`), jsondto/ui_schema/
    docs/example + RU-оверлей + фикстура. Core: `internal/skills/{manifest,remote}.go` (git/marketplace
    install-движок), `plugin.go` (`RunPluginCommand`, `MarketplaceStatus`), `Skill.Version`,
    loader dotfile-skip, gitws `Clone`/`Pull`. Plugin: `internal/agent/plugin_command.go` +
    `/plugin` в react.go; `BuiltinCommands` теперь и `plugin`. Auto-discovery: `internal/tools/load_skill.go`
    + `export.go` (гейт auto_discovery) + `toolsets.go` allowlist + `tooling/env.go` `LoadSkillBody` +
    `react.go`/`system_prompt.go` `loadSkillBody`. Плюс fix `f0911c9` (сброс empty-turn counter).
    HTTP: `skills_mgmt.go` расширен до 13 роутов (`s.sessionDefaultCWD()`, `invalidateSlashCache`,
    `reloadConfigFromDisk`), `docs/http-api.md`. CLI: `foxxycode skills add|sync|remove` + `plugin`.
    **Не** портирован транзитный `internal/tools/skills.go` (upstream его удаляет); `print_tree` в форке нет.
  - UI in-app marketplace — **ГОТОВО**: `settings/SkillsSection.tsx` (перепись 140→608, browse/install/
    sync/delete/update + версии), `Switch.tsx` (iOS-тумблер, подключён в `SchemaForm.tsx`),
    `installableMatches.ts`, `skills/commandRows.ts`, styles (~270стр). Билд + 680 UI-тестов зелёные.
    ⚠️ **i18n:** upstream-версия SkillsSection полностью на английском (ре-threading через `t()`/`en.ts`/
    `ru.ts` — отложенный follow-up; старые `settings.skills.*` ключи не используются).
- **Пост-волновые доработки (коммиты `f2f4682` + фикс визуального прогона) — ГОТОВО:**
  - `print_tree` — порт `internal/tools/fs/print_tree.go`, регистрация в fs-билтинах и
    plan-allowlist, тест, `docs/architecture.md`.
  - **Settings → Навыки переведён**: 36 новых `settings.skills.*` ключей в `en.ts`/`ru.ts`
    (46 используемых, полный паритет en/ru). Описание auto-discovery идёт через `tSchemaText`
    (schema-оверлей), иначе оставалось английским — поймано визуальным прогоном,
    регресс-тест `i18n/schemaSkillsLookup.test.ts`.
  - **Exhaustive OpenAPI**: все 12 зарегистрированных `/foxxycode/skills*` роутов описаны
    (10 путей) + схемы `SkillRow` (version/source/readonly), `SkillSyncResult`, `SkillUpdateList`.
  - **BDD**: `features/{plugin_command,remote_api,skills_marketplace}.feature` + харнессы
    (16 сценариев / 101 шаг), `workspace_switching.feature` перенесён в корневой `features/`
    (доделан `328bc25`); правило «happy path → features/, edge cases → unit-тесты» в `AGENTS.md`
    + `.claude/rules` с зеркалом в `.cursor/rules`.
  - **Визуальный прогон** на реальном бэкенде (`-tags http,ui`, изолированный home/config):
    13 вкладок настроек по-русски; Навыки полностью локализованы; «Движок сжатия» = `coddy`;
    slash-каталог отдаёт `compact` + `plugin`; compact-эндпоинт (404/валидация); env-чип
    (Local + Add remote + reachability); OpenAPI отдаёт 10 skill-путей и 5 схем; CLI
    `plugin marketplace list` / `skills list`.
  - **Остаток закрыт** (PR #7, коммит `ea7095d`, ветка `i18n/spa-remaining-english`): полный проход
    локализации SPA. 46 новых ключей (`composer.env.*`, `composer.workspace.*`,
    `composer.folderModal.*`, `env.banner.*`, `env.error.*`, `messages.compaction*`,
    `files.type.*`, `settings.providerApiKeyHint*`) в `en.ts`/`ru.ts`; переведены
    `EnvironmentChip`, `WorkspaceChips`, `WorkspaceFolderModal`, `EnvHealthBanner`,
    `remoteErrors`, `CompactionMessage`, `DiffView`, `UserMessage`, `fileTypeIcon`,
    заголовок «New chat» в `App.tsx` и fallback-ошибки настроек. Не переводятся намеренно:
    `AppErrorBoundary` (должен рендериться, даже если сломалась локализация),
    `compactionSummary.ts` (префиксы матчатся с текстом бэкенда) и технические
    placeholder-ы формата (`provider/model-id`, примеры URL). Защита от повторения хвоста —
    `i18n/messagesParity.test.ts` (паритет ключей и `{param}`-слотов en/ru) и
    `i18n/noHardcodedStrings.test.ts` (скан JSX-текста и label-атрибутов с аллоулистом).

---

## Последняя синхронизация

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-07-23 (перепроверка; новых коммитов нет) |
| **Синхронизировано до `upstream/main`** | `6666606` (2026-07-22) — на 2026-07-23 это по-прежнему HEAD ветки `upstream/main`, `git fetch upstream --prune` ничего не приносит |
| **Ближайший upstream-тег** | `0.9.43` |
| **Наш коммит-порт** | `f0a2506`, `60af986`, `305fc5a`, `3b3e812`, `0e75aa7` (ветка `sync/upstream-6666606`) |
| **Отложенные follow-up** | нет — все три закрыты: exhaustive OpenAPI для skill-роутов и BDD `skills_marketplace`/`plugin_command`/`remote_api` в `f2f4682`, i18n `SkillsSection.tsx` в `f2f4682`+`3d2fa15`, остальной английский в SPA — `ea7095d` (PR #7) |

### Что портировано в этой волне
- **Platform-aware shell** (upstream `2e979b7`) — новый пакет `internal/platform` (детект
  `pwsh → powershell → cmd` в Windows, `bash → sh` иначе); `run_command` больше не хардкодит
  `sh -c` и получает описание под конкретный шелл; `api_key_command` идёт через тот же шелл;
  блок `<environment_context>` (`<os>`/`<arch>`/`<shell>`) добавляется **вне** настраиваемого
  шаблона промпта. Затронуты `internal/tools/shell/run.go`, `internal/tools/export.go`
  (`NewRegistryForEnvironment`), `internal/agent/react.go`, `internal/agent/system_prompt.go`,
  `internal/config/providers.go`, `internal/config/ui_schema.go`.
- **Portable grep/glob** (upstream `d68d83c`) — новый `internal/tools/fs/search.go` с нативным
  Go-движком (`doublestar/v4`); `grep` использует системный `rg` при наличии (паттерн передаётся
  нетронутым после `--`) и падает в фолбэк иначе, `glob` тоже; `grepLineFilePath` понимает
  Windows-пути с буквой диска. Раньше `glob`/`grep` без `rg` в PATH просто падали.
- **Gateway в Docker** (upstream `204e3e9`) — build-тег `gateway` в `Dockerfile`/`docker-compose.dev.yml`,
  override `FOXXYCODE_COMMAND`, проброс `TELEGRAM_BOT_TOKEN`, разделы в `docs/docker.md`
  и `docs/gateway.md`.

### Особенности порта (fork-specific)
- `internal/tools/fs/fs_test.go` смержен вручную: upstream-тесты дописаны к локальным
  cp1251-тестам (`TestReadDecodesWindows1251` и др.), которые в upstream отсутствуют.
- `internal/tools/fs/read.go` оставлен локальным (слой `decodeText`/`encodeText`).
- **`grep` доработан поверх upstream под cp1251** (расхождение с upstream, сознательное):
  upstream-движок в `search.go` читал строки через `bufio.Scanner` как UTF-8, из-за чего
  кириллица в Windows-1251-файлах не находилась. Теперь `searchFileLines` декодирует файл
  через `decodeText`, а **non-ASCII паттерны маршрутизируются мимо системного `rg`**
  (`isASCIIPattern` в `grep.go`) — `rg` ищет по сырым байтам и такие файлы пропускает.
  ASCII-паттерны по-прежнему идут в `rg`. Регрессионные тесты: `TestGrepFindsCyrillicInWindows1251File`,
  `TestGrepNonASCIIPatternBypassesSystemRipgrep`, `TestGrepASCIIPatternStillUsesSystemRipgrep`.
- `system_prompt.go`: локальный `languageDirective` сохранён, upstream-блок окружения добавлен
  после него.
- Изменение описания `api_key_command` потребовало регенерации `ui-schema.json`-фикстуры
  (`UPDATE_UI_SCHEMA_FIXTURE=1`) и правки RU-оверлея `external/ui/src/ui/i18n/messages/schema.ru.ts`.

### Пропущено как уже покрытое / неактуальное
- `1585c72` (экранирование `$` в proxy-секретах) и `18c677c` (read за пределами EOF) — уже были
  в форке, файлы `internal/config/expand.go` и `internal/tools/fs/lines.go` побайтово совпадали
  с `upstream/main`.
- `b8cf8ce`, `a563294` (ветка `codex/windows-portable-tools` с `rg_tool.go`) — в upstream заменены
  на `search.go`; в дереве `upstream/main` файла `rg_tool.go` нет.
- Более ранние коммиты — см. волну до `55cc476` ниже по истории файла.

### Известные предсуществующие падения на Windows (не связаны с этой волной)
- `TestConcurrentPatchSessionMetaActivitySync` (`internal/session`) — флейк `rename … Access is denied`,
  воспроизводится и на чистом дереве.
- `kilocode-main/...` ломает `go build ./...` и `golangci-lint run ./...`; собирать/линтить
  по каталогам (`./cmd/... ./internal/... ./external/...`).
- `golangci-lint`: `bootstrapExampleConfig is unused` в `cmd/foxxycode/main.go` — тоже предсуществующее.

---

## Как обновить этот файл в следующий раз

1. `git fetch upstream --prune`
2. `git log --oneline --no-merges bc1afb9..upstream/main` — список кандидатов.
3. Портировать непортированное (ребренд `coddy → foxxycode`; см. `AGENTS.md` / память форка).
4. Прогнать гейты: `make test`, `make lint`, `npm --prefix external/ui run build:go`.
5. Обновить таблицу «Последняя синхронизация» выше на новый `upstream/main`.
