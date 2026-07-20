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
| **Дата** | 2026-07-20 |
| **Синхронизировано до `upstream/main`** | `bc1afb9` — *Merge pull request #56 from hijera/codex/fix-read-offset-eof* (2026-07-20) |
| **Ближайший upstream-тег** | `0.9.38` |
| **Наш коммит-порт** | (текущая волна) |

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
