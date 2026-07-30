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

## Волна `6666606 → 19754e8` (тег `0.9.43`) — ГОТОВО

9 не-merge коммитов (2026-07-22…23), портированы двумя коммитами на ветке
`claude/sync-foxxy-coddy-agents-c89f5c`.

### Коммит 1 — `usage_update` после компакции (upstream `29c58ae`)

- **ACP/HTTP**: новый `acp.UsageUpdate` (`sessionUpdate: "usage_update"`, `used`/`size`) —
  текущая занятость окна контекста; в `bridge.go` мапится в именованный SSE-эвент
  `usage_update`, описан в `openapi.go` и `docs/http-api.md`.
- **`internal/agent/context_usage.go`** (новый): `setContextBreakdown` пишет оценку в
  `stats.json` через `session.WriteSessionContextBreakdown` (провайдерские счётчики токенов
  сохраняются) и публикует эвент; `refreshConversationContextUsage` пересчитывает
  транскриптовые категории после компакции и после каждого persist-а сообщения.
- **`internal/session/manager_usage.go`** (новый): `restoreContextBreakdown` при загрузке
  сессии + `sendContextUsageUpdate` сразу после регистрации — переоткрытая сессия отдаёт
  сжатое окно, а не значение до компакции.
- **Turn-lock**: `acquireStubTurnLock` теперь `TryLock` и возвращает `ErrSessionTurnBusy`
  (как unix-flock), т.е. второй параллельный turn падает быстро, а не встаёт в очередь.
  Это путь `!unix`, т.е. **Windows** — прогнаны `internal/session` и `external/httpserver`.
- **SPA**: `withContextUsedTokens` в `contextUsage.ts`, ветка `usage_update` в
  `consumeComposerSse.ts`, `branchContextUsage` в обоих вызовах `consumeComposerSseReader`
  в `App.tsx`.

**Расхождения с upstream (осознанные, из-за двух движков компакции):**
- `llmVisibleMessages()` повторяет диспетчеризацию `buildMessages` (`MessagesForLLM` для
  `coddy`, фильтр `isLLMHistoryMessage` для `opencode`) и теперь кормит
  `computeContextBreakdown`. Это **починка живого бага форка**: `buildSystemPrompt` передавал
  нефильтрованный транскрипт, поэтому на дефолтном движке `coddy` (у него свёрнутые сообщения
  без флага) кольцо контекста не уменьшалось после компакции.
- Движок `opencode` теперь тоже публикует usage (после `ReplaceMessagesAndPersist`) — до этого
  он не отдавал `usage_update` вообще.
- Пересчитываются **и** `Conversation`, **и** `Summary`: компакция переносит текст из первой
  категории во вторую.
- `internal/skills/remote_test.go` (Windows-фикс `filepath.Abs`) не портирован — в форке уже
  есть эквивалентное исправление через `t.TempDir()`.

**Тесты:** upstream-овские BDD-шаги (ACP/HTTP/stats) + юнит-тесты, плюс два фичефайла форка,
которых в upstream нет: `features/context_compaction_engines.feature` (ACP-usage совпадает с
LLM-окном на **обоих** движках) и `features/context_compaction_restore.feature` (usage
выживает перезапуск сервера на обоих движках; `/compact` рекламируется только на `coddy`).
Оба проверены «наоборот»: без форковых правок сценарии падают.

### Коммит 2 — Tool-approval previews (upstream `2d5f2e7` + 6 follow-up)

Портировано как итоговое состояние (`git diff 6666606 upstream/main`), т.к. follow-up-ы только
дошлифовывают первый коммит, а три из них — только скриншоты.

- Новые `chat/permissionToolPreview.ts` и `chat/PermissionPromptPreview.tsx`: общий
  tool-specific preview для permission-гейта и раскрытой карточки инструмента.
- `PermissionPromptSection` — вопрос + один tool-id бейдж + preview; `ToolCallMessage` —
  статический preview + отдельная карточка `Результат`; единая кнопка
  `tool-overflow-toggle` (`Ещё…`/`Свернуть`) вместо текстовых ссылок в `ToolCallMessage`
  и `DiffView`; `rm`/`rmdir` добавлены в write-списки.

**Расхождения с upstream (осознанные):**
- **i18n:** весь новый слой переведён сразу (~30 ключей `prompts.permissionQuestion.*`,
  `prompts.permissionHeader.*`, `prompts.permissionMeta.*`, `messages.toolMore/toolLess/
  toolResultSection/patchPreviewAriaLabel/editPreviewAriaLabel`) в `en.ts` **и** `ru.ts`.
  Upstream-версия целиком английская.
- Upstream удаляет `permissionPromptTitle`; в форке он кормил desktop-тост, поэтому тост
  перевешен на `buildPermissionToolPreview(p).title` (тост и карточка теперь дают один текст),
  а ключи `prompts.permission{Fallback,ToolFallback,RunCommand}` удалены.
- `PermissionPromptSection` сохраняет форковый `submitPermissionChoice` (не inline `fetch`).
- `ToolCallMessage` сохраняет `useT()`, проп `sessionId` и ветку `BrowserAction`; новый preview
  выключен для browser-инструментов (`!isBrowserTool`), чтобы карточка скриншота осталась
  единственным рендерером.
- **CSS:** upstream-правило `[data-theme="light"] .chat-bottom:has(.composer-wrap-docked)` не
  проходит Chromium-104 гейт — переписано на форковый маркер `.chat-bottom--docked`
  (`themeCssContract.test.ts` матчится на него). У `.shell` `100svh` заменён на `100dvh`,
  потому что `check:compat` требует не-viewport fallback **непосредственно** перед `dvh`.
- `DiffView.tsx` и `chat/toolCallArgsDisplay.ts` остаются в дереве (как в upstream), хотя
  `ToolCallMessage` их больше не использует.

**Живая проверка** (реальный провайдер `neuraldeep`, ключ только через `NEURALDEEP_API_KEY`,
изолированные home/config вне репозитория):
- счётчик растёт на реальных turn-ах и совпадает с `usage_update` и `/stats` до токена;
- `/compact` на большой сессии: 6824 → 5052, значение из стрима == `/stats`, без перезагрузки;
- перезапуск бинаря: 5052 восстановлено из `stats.json` (и 4803 на `opencode`);
- авто-компакция срезает окно **внутри** одного стрима (5757 → 4918 на `coddy`,
  5757 → 4840 на `opencode` вместе с эвентами `compaction` start/done);
- кольцо в SPA живьём: 53.4 → 42.5 → 42.6 без перезагрузки;
- permission-карточки `write` / `run_command` / `edit` и раскрытая карточка инструмента —
  по-русски, `Ещё…`/`Свернуть` работает на усечённом результате (20 → 41 строка, viewport
  переключается на `--scroll`), консоль чистая.

**Отложено:** четыре PNG `docs/assets/screenshot-tool-previews*.png`. Решено снимать своим UI
(не тащить coddy-брендированные кадры upstream), но снять их в этой сессии не удалось — панель
браузера не отображалась (`screenshot` требует компоновки кадров), подключённого Chrome нет.
Записи в `docs/assets/INDEX.md` намеренно **не** добавлены, чтобы не ссылаться на отсутствующие
файлы. Когда панель будет открыта: снять `screenshot-tool-previews-{light,dark}.png` и
`screenshot-tool-previews-overflow-{light,dark}.png` и добавить секцию в `INDEX.md`.

---

## Волна `19754e8 → 96c04fb` (тег `0.9.44`) — ГОТОВО

5 не-merge коммитов (2026-07-25…26), 38 файлов. Портированы четырьмя коммитами.
Разведка перед портом показала, что база почти не разошлась: `internal/rules/{agents,select,list}.go`
**побайтово совпадали** с upstream на `19754e8`, сигнатуры `buildSystemPrompt`/`buildMessages`
идентичны, места патча в `.github/workflows/*` совпадали построчно.

### Коммит 1 — Loop guard (upstream `875bade`, наш `3c12ebd`)

Защита от зацикливания ReAct-турна. `internal/agent/loopguard.go` (портирован 1:1):
`streamRepeatDetector` следит за одним потоковым каналом (текст ответа или reasoning) по
скользящему нормализованному окну и срабатывает, когда хвост — это один и тот же фрагмент,
повторённый подряд; `toolRepeatDetector` считает подряд идущие одинаковые вызовы инструмента
по ключу «имя + канонизированные аргументы». При срабатывании повтор **вырезается** из
сохранённого сообщения ассистента (иначе `buildMessages` вернёт его модели и пересеет цикл).
Политика — nudge-then-stop: до `agent.loop_nudge_max` подсказок, затем `StopReasonRefused`.
Конфиг (секция `agent`, всё с дефолтами): `loop_guard`, `loop_tool_repeat_limit`,
`loop_stream_repeat_cycles`, `loop_nudge_max` — плюс полная обвязка jsondto/ui_schema/
schema_ui_defaults/`docs/config.schema.json`/`config-reference`/`config.md`/`config.example.yaml`/
`react-agent.md`/`architecture.md`/`README.md`.

**Расхождения с upstream (осознанные):**
- **Стрим-колбэк.** В форке текстовая ветка раздвоена (`else if chunk.TextDelta != ""` —
  whitespace-дельты не закрывают reasoning-часы). Проверка детектора вставлена **перед** обеими
  ветками, а срабатывающая дельта переиспускается с форковым правилом `markReasonEnd`
  (`strings.TrimSpace(delta) != ""`), а не безусловным `true`, как в upstream.
- **jsondto / schema_ui_defaults.** В форке нет upstream-хелперов `cloneBoolPtr`/`cloneIntPtr`/
  `boolPtr`/`intPtr`; следуем форковой конвенции — прямое копирование указателей (как
  `Compaction.KeepRecentTurns`) и локальные переменные + `&x`.
- **`NewAgent` вернул upstream-гард `if log == nil { log = slog.Default() }`.** Форк его потерял,
  из-за чего новые `a.log.Warn` паниковали на nil-логгере в тестах. Это чинит и **предсуществующие**
  nil-падения форка: `compact.go:218`, `context_usage.go:47`, `memory_hooks.go:78` дерефают
  `a.log` без проверки.
- **i18n:** текст остановки локализован в SPA — `chat/loopGuardNotice.ts` (матчинг по тексту
  бэкенда, тот же приём, что в `chat/compactionSummary.ts`) + ключи `messages.loopGuard*` в
  `en.ts`/`ru.ts`, подключено в `messages/SystemNoticeMessage.tsx` (локализованный текст идёт и в
  кнопку «копировать»). Бэкенд-строки остаются английскими: они же попадают в поле `error`
  HTTP/ACP-ответа. Строки config-схемы переведены в `schema.ru.ts`, фикстура `ui-schema.json`
  перегенерирована.

**Тесты:** `features/loop_protection.feature` (3 сценария — вырождение текста, вырождение
reasoning, повтор тулкола — против настоящего `Agent.Run` с фейковым провайдером),
`loopguard_test.go`, три кейса в `react_test.go` (исчерпание бюджета подсказок, выключенный
гард, различающиеся аргументы тулкола), `loopGuardNotice.test.ts`.

### Коммит 2 — Directory-scoped nested `AGENTS.md` (upstream `692a246`, наш `5d103cf`)

`AgentsProvider` делал из каждого вложенного `AGENTS.md` always-applied auto-правило без глобов,
поэтому `MatchAuto` пускал их все на первом же ходу. Теперь у каждого — свой `Rule.ScopeDir`
(новый `internal/rules/scope.go`: `PathUnderDir`/`PathsUnderDir`/`MatchScoped`), и тело попадает
в промпт только когда fs-тулкол или `file://`-путь целится в этот каталог или ниже; дальше
залипает на сессию. `run_command` не активирует ничего. Плюс кап 256 КБ на тело (как
`LoadProjectDocs`), колонка `GLOBS` → `ACTIVATES ON` в `foxxycode rules list`, фикс
`file:///C:/…` в `extractContextFiles` (Windows-путь `/C:/…` не матчился ни с одним scope и глобом).

**Форк-специфика:** хук `activateScopedRulesForToolCall` стоит в начале `executeToolCall` —
эта точка покрывает **и** ReAct-цикл, **и** форковый resume-after-permission
(`resume_permission.go:52` идёт через тот же `executeToolCall`). Активированное множество
пишется в `SetActiveAutoRules`, откуда его подхватывает форковый `buildRulesPromptMarkdown`
(`internal/agent/rules_prompt.go`) — правок в самом `rules_prompt.go` не потребовалось.
`docs/rules.md` не заменён блоком upstream (файл разошёлся: `.foxxyrules`, свои источники) —
внесены только смысловые правки.

**Тесты:** `features/agents_md_scoping.feature` (**проверен «наоборот»**: с `ScopeDir: ""`
сценарий падает на шаге «первый запрос несёт корневой AGENTS.md, но ни одного вложенного»),
`internal/rules/scope_test.go`, `internal/tools/fs/toolpaths_test.go`,
`internal/agent/rules_activation_test.go`, кап-кейс в `rules_test.go`.

### Коммит 3 — pre-commit-гейт (upstream `1570a19` + `b9dd5cb`, наш `9f9b476`)

`.githooks/pre-commit` → `scripts/checks.sh`; `make hooks` включает его один раз на клон.
По умолчанию только `make lint`; docs-only коммиты гейт пропускают.

**Форк-специфика:** переменные `CODDY_HOOK_*` → **`FOXXYCODE_HOOK_*`** (`LINT`/`TESTS`/`SKIP`).
Добавлен **`.gitattributes`** с `*.sh text eol=lf` и `.githooks/* text eol=lf`: форк разрабатывается
на Windows с `core.autocrlf=true`, который иначе переписал бы оба скрипта в CRLF при checkout и
сломал строку shebang. Exec-биты выставлены явно (`git update-index --chmod=+x`) — Windows не
сообщает filemode. В `AGENTS.md` отмечено, что `FOXXYCODE_HOOK_TESTS=full` идёт через `make test`,
чей шаг `ui-build` на этой машине падает, поэтому практический opt-in — `fast`.

**Проверено:** хук прогнан на обоих состояниях индекса (docs-only пропускает гейт; staged
Makefile/скрипт запускает `make lint` — 0 issues); ручки `checks.sh` проверены напрямую
(skip, lint off, неизвестное `TESTS` → выход 2).

### Коммит 4 — фикс GitHub Actions (upstream `0191068`) + этот файл

`allow-unsafe-pr-checkout: true` в `actions/checkout` трёх workflow (`docker-build-push`,
`release-binaries`, `tag-on-merge`) — под `pull_request_target` checkout иначе отказывается брать
tag, производный от форка. Патч применился как есть; гейты безопасности, на которые ссылаются
комментарии (шаг «Ensure tag is on main», условие `merged == true`), в форке присутствуют.

---

## Волна `96c04fb → 6d46afe` (теги `0.9.45`–`0.9.48`) — ГОТОВО

Волна разбита на четыре независимых upstream-блока и адаптирована к текущей архитектуре
FoxxyCode.

### Windows shell и PowerShell (`96c04fb → 5c59782`)

- Добавлен platform-aware вывод команд и корректная сборка PowerShell-скриптов:
  многострочные команды, специальные символы, native stderr/stdout и exit code больше не
  зависят от синтаксиса POSIX shell.
- Маркер успешного выполнения переименован в `$__foxxycodeOK`.
- `api_key_command` использует ту же платформенную shell-обвязку.
- Сохранены форковые особенности: CP1251 read/grep и Ask-mode shell validator.

### Управление MCP (`5c59782 → f664884`)

- Портированы конфиг/JSON DTO, lifecycle подключений, удалённые MCP-серверы, HTTP management API,
  OpenAPI, BDD/unit-тесты и отдельная вкладка MCP в настройках SPA.
- Управляемые клиенты пересоздаются при смене project CWD; ACP-клиенты при этом сохраняются.
  Probe-cache также разделён по рабочему каталогу.
- Сохранены форковые фильтры инструментов по режиму (`agent`/`plan`/`docs`/`ask`) и динамическая
  фильтрация конфигурации.
- Весь новый UI локализован через `t()`; EN/RU parity и запрет hardcoded-строк проверены тестами.

### Codex OAuth/provider (`f664884 → 171854c`)

- Добавлены `foxxycode codex login/logout/status`, device OAuth, provider `codex`, загрузка моделей,
  reasoning replay и HTTP/UI-поток авторизации.
- Codex добавлен в first-run onboarding: вместо API-ключа показывается ChatGPT OAuth, а каталог
  моделей читается по server-side credential до сохранения нового провайдера.
- Внутренние имена ребрендированы (`FOXXYCODE_CODEX_BASE_URL`, managed source `foxxycode`), но
  внешний контракт Codex CLI намеренно сохранён: `CODEX_HOME`, `~/.codex/auth.json`,
  source `codex_cli`.
- Сохранены FoxxyCode `MaxContextTokens`, NeuralDeep-настройки и обе реализации compaction.
- Автогенерация заголовка в продукте не отключалась; только изолирована в сценарных тестах с
  жёстко запрограммированным провайдером, чтобы фоновый title-request не сдвигал их очередь.

### Result eviction и output limits (`171854c → 6d46afe`)

- Добавлена проекция истории перед отправкой LLM: устаревшие крупные результаты инструментов
  удаляются, а `keep_result` закрепляет нужное наблюдение.
- Добавлены настраиваемые лимиты строк/байтов для встроенных и MCP-инструментов с UTF-8-safe
  усечением; схемы, примеры конфигурации и UI schema синхронизированы.
- `keep_result` доступен во всех форковых read-capable режимах, включая `docs` и `ask`.
- Сохранены обе реализации compaction: coddy-проекция учитывает записи и pins из оставляемого
  хвоста до выбора старой головы.
- Мутации SVN консервативно инвалидируют закреплённые чтения рабочей области; добавлены
  регрессионные тесты. Слой CP1251 в read/grep сохранён.

### Проверка форковых регрессий

Перед портом просмотрены последние изменения FoxxyCode из PR #18–#20. Сохранены:

- `session_busy`, turn-lock, отмена и восстановление долгого стриминга;
- `X-Accel-Buffering: no`, быстрый список сессий и порядок загрузки transcript/list/stats;
- форковая status line и остальные секции настроек SPA.

Покрытие этой волны включает feature-сценарии для PowerShell, MCP, Codex OAuth и result eviction,
unit-тесты конфигурации/компакции/лимитов, HTTP-регрессии и UI i18n/schema-тесты.

---

## Последняя синхронизация

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-07-29 |
| **Синхронизировано до `upstream/main`** | `6d46afe` (2026-07-29) |
| **Ближайший upstream-тег** | `0.9.48` |
| **Наш коммит-порт** | подготовлен на ветке `codex/sync-upstream-6d46afe` |
| **Отложенные follow-up** | нет |

---

## Предыдущая синхронизация (`6666606 → 19754e8`)

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-07-25 |
| **Синхронизировано до `upstream/main`** | `19754e8` (2026-07-24) |
| **Ближайший upstream-тег** | `0.9.43` |
| **Наш коммит-порт** | `323ba32` (волна A: `usage_update`) + волна B (tool previews) — ветка `claude/sync-foxxy-coddy-agents-c89f5c` |
| **Отложенные follow-up** | четыре PNG `docs/assets/screenshot-tool-previews*.png` (см. выше) |

---

## Волна `bc1afb9 → 6666606` — предыдущая синхронизация

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-07-23 (перепроверка; новых коммитов нет) |
| **Синхронизировано до `upstream/main`** | `6666606` (2026-07-22) |
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
2. `git log --oneline --no-merges 6d46afe..upstream/main` — список кандидатов.
3. Портировать непортированное (ребренд `coddy → foxxycode`; см. `AGENTS.md` / память форка).
4. Прогнать гейты: `make test`, `make lint`, `npm --prefix external/ui run build:go`.
5. Обновить таблицу «Последняя синхронизация» выше на новый `upstream/main`.
