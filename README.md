<p align="center">
  <strong>Русский</strong> | <a href="README.en.md">English</a>
</p>

<p align="center">
  <a href="https://go.dev/doc/go1.25"><img src="https://img.shields.io/badge/go-1.25+-00ADD8?logo=go&logoColor=white" alt="Go 1.25+" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/hijera/foxxycode-agent" alt="Лицензия MIT" /></a>
  <a href="https://github.com/hijera/foxxycode-agent/actions/workflows/tests-on-pr.yaml"><img src="https://github.com/hijera/foxxycode-agent/actions/workflows/tests-on-pr.yaml/badge.svg" alt="Тесты для PR" /></a>
  <a href="https://agentclientprotocol.com/"><img src="https://img.shields.io/badge/ACP-harness-9333EA" alt="Среда ACP" /></a>
  <img src="https://img.shields.io/badge/distroless%20ready-252525" alt="Готово для distroless" />
  <img src="https://img.shields.io/badge/single%20binary-252525" alt="Один исполняемый файл" />
</p>

<h1 align="center">FoxxyCode Agent</h1>

<p align="center">
  <strong>Полноценный универсальный агент в одном статическом исполняемом файле на Go.</strong><br />
  ReAct, инструменты для файловой системы и командной строки, MCP, навыки, опциональный OpenAI-совместимый API со встроенным интерфейсом, планировщик и долговременная память.<br />
  Удобный для IDE форк, который легко адаптировать к выбранному редактору.
</p>

> **Foxxy Agent основан на [coddy-agent](https://github.com/coddy-project/coddy-agent)** проекта Coddy (MIT).
> Этот форк сохраняет архитектуру исходного проекта и совместимость с его обновлениями, но меняет
> оформление дистрибутива (репозиторий, имя исполняемого файла и релизы) и упрощает адаптацию к IDE.

**Что FoxxyCode добавляет к coddy-agent** (см. [полный список](docs/vs-coddy.md)):

- **Нативное настольное окно (WebView2)** с системными уведомлениями, звуковым сигналом и пошаговым знакомством при первом запуске
- **Глубокая интеграция с IDE** — контекст открытых файлов (`<foxxycode_ide_context>`), отслеживание терминала (`@terminal`), упоминание файлов перетаскиванием, выбор папки проекта и нативные встроенные diff в IntelliJ
- **Интерактивный браузерный инструмент** — управляет настоящим Chrome через chromedp и возвращает модели снимки экрана (`-tags=browser`); подробнее в разделе [браузерного инструмента](docs/browser-tool.md)
- **Автоматическое сжатие контекста** — по умолчанию автоматически суммирует длинные диалоги
- **Русская локализация настроек** и полный ребрендинг дистрибутива в `foxxyCode`

![Главное окно FoxxyCode](docs/assets/foxxycode1.png)

FoxxyCode — совместимая с distroless **среда выполнения агента**: её можно помещать в минимальные образы (`scratch`, `distroless`, рабочие каталоги только для чтения), не устанавливая полноценную системную оболочку. Уровень среды (ACP RPC, сессии, промпты, провайдеры) не меняется, если ограничить набор инструментов или управлять агентом из автоматизации вместо IDE. Архитектура также рассчитана на **контейнерные кластеры** — множество экземпляров FoxxyCode в Docker с заданными оркестратором ограничениями, корневой ФС только для чтения и подключённым рабочим каталогом. При этом сохраняется **полный контроль над каждым контейнером**, как в системах класса agent OS или swarm-агентов, а не в едином общем пуле чатов.

## Содержание

- [Возможности](#возможности)
- [Быстрый старт](#быстрый-старт)
  - [Установка](#установка)
  - [Другие способы установки](#другие-способы-установки)
  - [Теги сборки](#теги-сборки)
  - [Docker](#docker)
  - [Пути (`FOXXYCODE_HOME`, `FOXXYCODE_CWD`)](#пути-foxxycode_home-foxxycode_cwd)
  - [Настройка](#настройка)
- [Обновление](#обновление)
- [Режимы работы](#режимы-работы)
- [Интеграция с редакторами и IDE](#интеграция-с-редакторами-и-ide)
- [Правила](#правила)
- [Навыки](#навыки)
- [Интеграция MCP-серверов](#интеграция-mcp-серверов)
- [Шлюз мессенджеров](#шлюз-мессенджеров)
- [Справочник по конфигурации](#справочник-по-конфигурации)
- [Архитектура](#архитектура)
- [Документация](#документация)
- [Примеры (ACP через stdio)](#примеры-acp-через-stdio)
- [Постоянные сессии](#постоянные-сессии)
- [Разработка](#разработка)
- [Лицензия](#лицензия)

## Возможности

- **Среда выполнения — прежде всего** — ACP-сервер, жизненный цикл сессий, промпты, LLM-бэкенды, объединение MCP и готовый для distroless исполняемый файл
- **Цикл ReAct** — LLM чередует рассуждение, действие (вызов инструментов) и наблюдение за результатами; профиль кодинг-агента доступен из коробки
- **Три режима работы** — `agent` (полный доступ к инструментам), `plan` (планирование без выполнения кода) и `docs` (только Markdown-документация)
- **Правила** — автоматически находит **`.cursor/rules/`**, **`.foxxycode/rules/`**, **`.claude/rules/`**, **`.codex/rules/`** и вложенные **`**/AGENTS.md`** (соглашение [agents.md](https://agents.md/)) в рабочем каталоге сессии; подробнее в разделе [Правила](docs/rules.md)
- **Навыки** — slash-команды и пакеты **`SKILL.md`** из **`skills.dirs`** (по умолчанию: **`~/.agents/skills`**, **`~/.foxxycode/skills`**, **`${CWD}/.foxxycode/skills`**; более поздний каталог имеет приоритет); подробнее в разделе [Навыки](docs/skills.md)
- **Интеграция MCP-серверов** — подключение любого MCP-сервера для доступа к дополнительным инструментам
- **Несколько LLM-провайдеров** — OpenAI, Anthropic, Ollama и любой OpenAI-совместимый API
- **Мультимодальность и вложения** — изображения и файлы можно прикреплять через поле ввода (📎), если в настройках модели указано `multimodal: true`; файлы сохраняются в `~/.foxxycode/sessions/<id>/assets/`, передаются в контекст агента и отображаются в сообщении пользователя
- **Уровень рассуждения** — для моделей с рассуждением (gpt-5, серия o, модели Claude с thinking) выпадающий список в поле ввода задаёт уровень (`minimal`/`low`/`medium`/`high`), который преобразуется в OpenAI `reasoning_effort` или Anthropic extended-thinking `budget_tokens`; поддержка определяется автоматически по идентификатору модели и настраивается для каждой модели — см. [Настройку](docs/config.md)
- **Протокол ACP** — FoxxyCode работает как **ACP-сервер** (`foxxycode acp`); его можно подключить к редактору или скрипту с ACP-клиентом (см. [Интеграцию с редакторами и IDE](#интеграция-с-редакторами-и-ide))
- **Удалённое выполнение по SSH** — встроенный инструмент `ssh_run_command` выполняет команды на удалённых узлах через реализацию SSH на чистом Go, без внешнего исполняемого файла; аутентификация использует SSH-агент (`SSH_AUTH_SOCK`) или ключи из `~/.ssh` — см. [Настройку](docs/config.md#ssh-remote-execution)
- **Шлюз мессенджеров** — опциональный адаптер Telegram-бота (`-tags gateway.telegram`), отдельные сессии пользователей, режимы изоляции групп и ACL администраторов; архитектуру можно расширить для Discord, Slack и других сервисов — см. [Шлюз мессенджеров](docs/gateway.md)

## Интеграция с редакторами и IDE

FoxxyCode работает как **ACP-сервер** (`foxxycode acp`). **Obsidian**, **VS Code**, **Zed**, скрипты и встроенный интерфейс **`foxxycode http`** выступают клиентами и используют одни и те же сессии в **`FOXXYCODE_HOME`**, если настроены на общий домашний каталог.

Указывайте в клиентах **абсолютный путь** к исполняемому файлу, не полагаясь на `PATH`: некоторые среды запускают агента через `cmd /c` или `sh -c` без пользовательского `PATH` (в Windows: `%LOCALAPPDATA%\Programs\foxxycode\foxxycode.exe`; см. [`docs/install.md`](docs/install.md#windows)).

Описание протокола: **`docs/acp-protocol.md`**. Примеры среды: **`examples/acp/`**.

## Быстрый старт

### Установка

**Сборка из исходников** (рекомендуется; требования перечислены в разделе «Другие способы установки»):

```bash
git clone https://github.com/hijera/foxxycode-agent
cd foxxycode-agent
make build TAGS="http ui scheduler memory"
make install   # копирует build/foxxycode в ~/.local/bin или /usr/local/bin
```

В **Windows** (или без GNU Make) используйте интерактивный мастер:
**`python scripts/build.py`** — русскоязычное консольное меню для сборки CLI, плагина IntelliJ, VS Code VSIX, выбора тегов и целевых платформ. Подробнее в **[`docs/build.md`](docs/build.md#interactive-build-wizard)**.

Можно также скачать архив для своей платформы из **[GitHub Releases](https://github.com/hijera/foxxycode-agent/releases)** и добавить исполняемый файл **`foxxycode`** в **`PATH`**.

Создайте начальную конфигурацию: **`mkdir -p ~/.foxxycode && cp config.example.yaml ~/.foxxycode/config.yaml`**.

> **Windows.** Поместите исполняемый файл в `%LOCALAPPDATA%\Programs\foxxycode\foxxycode.exe`; конфигурация и сессии хранятся в `%USERPROFILE%\.foxxycode\` (используйте `$env:USERPROFILE`, а не `$HOME`). Терминал, открытый во время установки, не увидит обновлённый `PATH` — откройте новый или обновите переменную в текущем. Подробнее: [`docs/install.md`](docs/install.md#windows).

Затем укажите ключ провайдера в **`~/.foxxycode/config.yaml`** (или переменную среды **`OPENAI_API_KEY`**) и запустите **`foxxycode http`** для веб-интерфейса либо **`foxxycode acp`** для клиента редактора.

**Docker** — тот же полный исполняемый файл доступен в образе **`ghcr.io/hijera/foxxycode-agent`**: выполните **`docker compose up -d`** (см. [Docker](#docker)).

В дальнейшем обновляйтесь командой **`foxxycode update -y`** (см. [Обновление](#обновление)).

<a id="другие-способы-установки"></a>
<details>
<summary><strong>Другие способы установки</strong> (сборка из исходников, Go install, ручная сборка)</summary>

**Требования для сборки**

- **Go** — та же минорная версия, что указана в [`go.mod`](go.mod) (сейчас **1.25**).
- **Git** — Makefile использует его для встраивания номера версии.
- **Node.js / npm** — нужны только при сборке с тегами **`http`** и **`ui`** (Makefile запускает **`ui-build`** для встраиваемых ресурсов).

**Установка через Go (минимальный модуль без тегов `http` / `ui`)**

```bash
go install github.com/hijera/foxxycode-agent/cmd/foxxycode@latest
```

Примечание: `go install` называет исполняемый файл по каталогу пакета (**`foxxycode`**). Чтобы получить **`foxxycode http`**, встроенный SPA, планировщик и память, используйте **архив релиза** или **соберите проект из исходников** (см. [Установку](#установка)).

**Ручной вызов `go build`**

Если **`TAGS`** содержит **`http`** и **`ui`**, сначала выполните **`make ui-build`**.

```bash
make ui-build
VERSION="$(make -s print-version)"
go build -tags=http,ui,scheduler,memory \
  -ldflags "-X github.com/hijera/foxxycode-agent/internal/version.Version=${VERSION}" \
  -o build/foxxycode \
  ./cmd/foxxycode/
```

Минимальный исполняемый файл **только с ACP**: **`make build`** (без тегов **`http`**, UI, планировщика и памяти).

**Настольное приложение Windows** (GUI на WebView2; запускается двойным щелчком по **`foxxycode-desktop.exe`**):

```bash
make build-desktop
```

Настольное приложение открывает проекты как папки: кнопка проекта в заголовке чата открывает нативный диалог выбора каталога Windows, новые чаты запускаются в выбранной папке, а недавние проекты сохраняются в `~/.foxxycode/projects.json` (**`GET/PUT /foxxycode/project`**, **`GET /foxxycode/projects/recent`**). Если **`-cwd`** не указан явно, при запуске восстанавливается последний открытый проект.

Справочник по сборке: **[`docs/build.md`](docs/build.md)**.

</details>

**`foxxycode -v`** выводит встроенную версию. **`foxxycode acp --help`** показывает параметры ACP (**`--home`**, **`--cwd`**, **`--config`** и другие).

### Теги сборки

В переменной **`TAGS`** для **`Makefile`** используйте **пробелы** (**`make build TAGS="http ui scheduler memory"`**), а в **`go build`** — **запятые** (**`-tags=http,ui,scheduler,memory`**).

| Тег | Что включает | Документация |
|-----|--------------|--------------|
| **`memory`** | Компонент долговременной памяти (**`memory.enabled`** в YAML); вместе с **`http`** — REST для памяти сессии в **`/foxxycode/sessions/{id}/memory/*`** | [`external/memory/README.md`](external/memory/README.md) |
| **`http`** | **`foxxycode http`**, REST-шлюз, **`/docs`**, **`/openapi.yaml`** | [`docs/http-api.md`](docs/http-api.md) |
| **`ui`** | Встроенный SPA на **`/`** (требует **`http`**) | [`docs/ui.md`](docs/ui.md), [`DESIGN.md`](DESIGN.md) |
| **`scheduler`** | Демон планировщика и инструменты **`foxxycode_scheduler_*`**; вместе с **`http`** — REST **`/foxxycode/scheduler`** | [`docs/scheduler.md`](docs/scheduler.md), [`external/scheduler/README.md`](external/scheduler/README.md) |
| **`browser`** | Интерактивные браузерные инструменты (**`foxxycode_browser_*`**: navigate/click/fill/hover/scroll/screenshot/evaluate), управляющие локальным Chrome/Chromium через chromedp; модель видит снимки страницы (**`browser.enabled`** в YAML) | [`docs/browser-tool.md`](docs/browser-tool.md) |
| **`gateway.telegram`** | Адаптер Telegram-бота — подкоманда **`foxxycode gateway`**, отдельные сессии пользователей и контроль доступа | [`docs/gateway.md`](docs/gateway.md) |
| **`gateway`** | Все адаптеры мессенджеров (надмножество `gateway.telegram`; позволяет добавлять Discord и Slack без изменений ядра) | [`docs/gateway.md`](docs/gateway.md) |
| **`desktop`** | Настольное приложение Windows на WebView2 (**`foxxycode desktop`** / **`foxxycode-desktop.exe`**; требует **`http`**, **`ui`** и Windows) | [`docs/build.md`](docs/build.md#desktop-windows-webview2) |

Расширенное описание и соответствие Docker-сборке: **[docs/build.md](docs/build.md)**.

### Docker

Образы релизов публикуются в **[GitHub Container Registry](https://github.com/hijera/foxxycode-agent/pkgs/container/foxxycode-agent)** под именем **`ghcr.io/hijera/foxxycode-agent`** (теги **`latest`**, **`X.Y.Z`** и другие; платформы **linux/amd64** и **linux/arm64**). Для каждого SemVer-тега также создаются архивы **GitHub Release** для Linux, Windows, macOS Intel и Apple Silicon; подробнее в **[docs/build.md](docs/build.md#release-binaries-ci)**. Стандартный образ включает **`http`**, **`ui`**, **`scheduler`** и **`memory`** — тот же набор функций, что и **`make build TAGS="http ui scheduler memory"`**.

**1. Конфигурация и рабочий каталог** (из корня репозитория или другого каталога, в котором хранится **`config.yaml`**):

```bash
cp config.example.yaml config.yaml
mkdir -p workspace foxxycode_home
# Отредактируйте config.yaml: нужен как минимум api_key одного провайдера
# (либо передайте OPENAI_API_KEY и другие переменные через Compose)
```

**2. Запуск через Compose** (загрузка опубликованного образа без локальной сборки):

```bash
docker compose pull
docker compose up -d
```

Чтобы **собрать образ локально**, используйте **`docker-compose.dev.yml`**: **`docker compose -f docker-compose.dev.yml up -d --build`**.

**3. Откройте встроенный интерфейс** в браузере на хосте:

```text
http://127.0.0.1:12345/
```

SPA доступен по **`GET /`** после запуска **`foxxycode http`**. Выберите **модель** в поле ввода (YAML-бэкенды из **`GET /v1/models`**), режим **agent**, **plan** или **docs**, затем отправьте сообщение. Интерфейс создаст сессию и начнёт потоковую передачу ответа через **`POST /v1/responses`**. Файловые и консольные инструменты агента работают в подключённом каталоге (**`./workspace`** → **`/workspace`** внутри контейнера). Редактор YAML в реальном времени: **`http://127.0.0.1:12345/#/settings`**.

Проверка без браузера: **`curl -sS http://127.0.0.1:12345/v1/models | head`**.

HTTP-интерфейс **не защищён авторизацией** — открывайте порт **12345** только в доверенных сетях. Все параметры Compose, тома и теги CI-образов описаны в **[docs/docker.md](docs/docker.md)**. Скрипт быстрой проверки: **`examples/httpserver/docker.sh`**.

### Пути (`FOXXYCODE_HOME`, `FOXXYCODE_CWD`)

- **`FOXXYCODE_HOME`** (или **`foxxycode acp --home`**) — каталог состояния агента. По умолчанию **`~/.foxxycode`**. Процесс создаёт в нём **`sessions/`** и **`skills/`**. Стандартный путь к конфигурации — **`$FOXXYCODE_HOME/config.yaml`**.
- **`FOXXYCODE_CWD`** (или **`foxxycode acp --cwd`**) — стандартный рабочий каталог сессии, когда `session/new` передаёт пустое значение **`cwd`**. По умолчанию это текущий каталог процесса при запуске. Если редактор передаёт путь в **`session/new`**, используется именно он.

### Настройка

По умолчанию **`FOXXYCODE_HOME`** указывает на **`~/.foxxycode`**. Если не задана переменная **`FOXXYCODE_CONFIG`** и не передан параметр **`--config`**, основным файлом конфигурации будет **`config.yaml`** в **`$FOXXYCODE_HOME/config.yaml`**.

Скопируйте пример и отредактируйте его:

```bash
mkdir -p ~/.foxxycode && cp config.example.yaml ~/.foxxycode/config.yaml
```

Если **`$FOXXYCODE_HOME/config.yaml`** отсутствует, загрузчик может использовать **`config.yaml`** из текущего рабочего каталога процесса — это удобно при запуске из клона репозитория. Подробнее в **`docs/config.md`**.

**Провайдеры и модели**

- **`providers`** — именованные бэкенды (**`type`**: **`openai`** для OpenAI и OpenAI-совместимых HTTP API, **`anthropic`** для Anthropic, **`neuraldeep`** для хаба NeuralDeep). Поле **`name`** должно состоять из ASCII-букв, цифр, дефиса или подчёркивания и начинаться с буквы: оно становится префиксом идентификатора модели. В каждой записи есть **`api_key`** (строка, выражение **`${ENV}`**, раскрываемое при чтении файла, или пустое значение для чтения **`NAME_API_KEY`** из среды в момент вызова LLM; **`NAME`** строится из `providers[].name` в верхнем регистре с заменой дефисов на подчёркивания) и опциональное **`api_base`**, если используется нестандартный API. Для **`neuraldeep`** поле **`api_base`** игнорируется: адрес всегда равен **`https://api.neuraldeep.ru/v1`**, поэтому нужен только **`api_key`**.
- **`models`** — доступные для выбора модели. Строка **`model`** имеет вид **`<provider_name>/<api_model_id>`**, где **`provider_name`** совпадает с `providers[].name`. Доступные параметры: **`max_tokens`**, **`temperature`** и опциональный **`max_context_tokens`**.
- **`agent`** — поле **`model`** выбирает стандартную модель ReAct и должно совпадать с одной из записей **`models[].model`**. Параметры **`max_turns`** и **`max_tokens_per_turn`** ограничивают один пользовательский запрос.

Пример с провайдером **`openai`** и моделью **`gpt-5.4-mini`**; храните секреты в переменных среды, а не в Git:

```yaml
providers:
  - name: openai
    type: openai
    api_key: "${OPENAI_API_KEY}"

models:
  - model: "openai/gpt-5.4-mini"
    max_tokens: 400000
    temperature: 0.2

agent:
  model: "openai/gpt-5.4-mini"
  max_turns: 35
  max_tokens_per_turn: 128000
```

Экспортируйте переменную, на которую ссылается YAML:

```bash
export OPENAI_API_KEY="sk-..."
```

Другие варианты (Anthropic, Ollama, нестандартное значение **`api_base`** и значения из переменных среды по умолчанию) описаны в **`config.example.yaml`** и **[docs/config.md](docs/config.md)**.

## Обновление

Официальные CLI-сборки публикуются в **[GitHub Releases](https://github.com/hijera/foxxycode-agent/releases)** (например, **`foxxycode_0.9.3_linux_amd64.tar.gz`**). Каждый релиз содержит полный набор функций сборки **`make build TAGS="http ui scheduler memory"`**.

Команда **`foxxycode update`** загружает архив для текущей ОС и архитектуры и заменяет запущенный исполняемый файл с разрешением символических ссылок. Обычно так обновляют установку после **`make install`** (**`~/.local/bin/foxxycode`**) или локальный артефакт командой **`./build/foxxycode update`**.

**1. Посмотрите, какая версия запущена**

```bash
which foxxycode
foxxycode -v
```

**2. Проверьте наличие нового релиза**

```bash
foxxycode update --check
```

Код завершения **0** означает, что установлена последняя опубликованная версия **`X.Y.Z`** или новее. Код **1** означает, что доступен новый релиз.

**3. Установите обновление**

```bash
foxxycode update          # спрашивает [y/N]
foxxycode update -y       # без подтверждения
```

**4. Проверьте результат**

```bash
foxxycode -v
foxxycode http --help     # только если сборка содержит -tags=http (как официальные релизы)
```

**Основные параметры**

| Параметр | Назначение |
|----------|------------|
| **`--check`** | Только проверить наличие обновления, ничего не загружая. |
| **`-y`** / **`--yes`** | Установить без подтверждения. |
| **`--version X.Y.Z`** | Установить конкретный релиз, а не только последний. |
| **`--repo owner/name`** | Использовать другой GitHub-репозиторий (по умолчанию **`hijera/foxxycode-agent`**). |

**Примечания**

- Обновляйте именно тот файл, который собираетесь использовать. Если `which foxxycode` указывает на `~/.local/bin/foxxycode`, запускайте `foxxycode update` из этой установки, а не другую копию из **`PATH`**.
- **`$FOXXYCODE_HOME`** с конфигурацией, сессиями и навыками не изменяется; заменяется только исполняемый файл.
- Для сборки из исходников или изменения тегов используйте **`make build`**. Для контейнеров — **`docker compose pull`**. Таблицы платформ, ограничения и другие способы обновления приведены в **[docs/update.md](docs/update.md)**.

## Режимы работы

### Режим Agent (по умолчанию)

Режим полноценного выполнения задач. Агенту доступны все инструменты:

- чтение и запись файлов;
- выполнение команд оболочки с запросом разрешения;
- поиск по кодовой базе;
- вызов инструментов MCP-сервера.

Лучше всего подходит для генерации кода, рефакторинга, отладки и реализации новых функций.

### Режим Plan

Режим планирования и документирования с ограниченным набором инструментов:

- чтение и поиск в рабочем каталоге;
- использование оболочки и настроенных MCP-инструментов для исследования;
- сохранение и загрузка проектных планов специальными инструментами.

Когда план готов, самостоятельно переключитесь в режим **agent** для полноценной работы с инструментами и реализации.

Лучше всего подходит для архитектурного планирования, спецификаций, проектной документации и ревью кода.

### Режим Docs

Режим сопровождения документации с закрытым набором инструментов:

- запрос на ревью ничего не изменяет, если пользователь явно не попросил обновить файлы;
- можно читать и искать в рабочем каталоге, а затем сверять утверждения с реализацией и наблюдаемыми результатами тестов;
- файлы `.md` внутри рабочего каталога сессии можно создавать и редактировать защищёнными инструментами **`docs_write`** и **`docs_edit`**;
- недоступны оболочка, MCP, общие операции изменения файлов, инструменты планов и списков задач.

Markdown-инструменты запрещают выход за пределы рабочего каталога и переход по символическим ссылкам, защищают **`internal/prompts/`**, требуют явного согласия перед перезаписью существующего файла и принимают только непустое уникальное точное совпадение при точечном редактировании, кроме случаев намеренной замены всех совпадений. Для изменения исходного кода или конфигурации переключитесь в режим **agent**.

Лучше всего подходит для синхронизации README и `docs/` с кодом, обновления руководств оператора и описания API.

Выберите режим в настройках сессии редактора или через **`session/set_config_option`**.

## Правила

Если **`rules.auto_discover`** включён, правила проекта, передаваемые через **`{{.Rules}}`**, автоматически находятся внутри рабочего каталога сессии в **`.foxxycode/rules`**, **`.cursor/rules`**, **`.claude/rules`**, **`.codex/rules`** и вложенных **`**/AGENTS.md`** (соглашение [agents.md](https://agents.md/); корневой `AGENTS.md` передаётся отдельно как вводная документация проекта). Подробнее в **[`docs/rules.md`](docs/rules.md)**.

Файлы правил часто используют frontmatter в стиле Cursor, например:

```markdown
---
description: "Стандарты кода Go"
globs: ["**/*.go"]
alwaysApply: false
---

Пишите все комментарии на английском языке.
Для оборачивания ошибок используйте fmt.Errorf("context: %w", err).
```

## Навыки

Slash-команды и пакеты **`SKILL.md`**, передаваемые через **`{{.Skills}}`**, расширяют агента предметными знаниями и специализированными процессами.

**Стандартные каталоги (от низшего к высшему приоритету):**

| Приоритет | Путь | Назначение |
|-----------|------|------------|
| низший | `~/.agents/skills/` | Общие навыки, установленные через `npx skills` или `npx skillsbd` и доступные всем агентам |
| ↑ | `~/.foxxycode/skills/` | Навыки FoxxyCode; могут содержать символические ссылки на `~/.agents/skills/` |
| высший | `${CWD}/.foxxycode/skills/` | Навыки проекта, переопределяющие одноимённые навыки из предыдущих каталогов |

Если навык с одним именем встречается в нескольких местах, более поздний каталог имеет приоритет.

**Поиск и установка навыков:**

- **[skills.sh](https://skills.sh)** — реестр сообщества; установка: `npx skills add <owner/repo@skill>`
- **[neuraldeep.ru/skills](https://neuraldeep.ru/skills)** — реестр skillsbd, отобранный для FoxxyCode; установка: `npx skillsbd install <name>`
- **Настройки → Навыки** в веб-интерфейсе (`foxxycode http`) — просмотр и установка из реестра skillsbd прямо в браузере

**CLI:**

```bash
foxxycode skills list              # список установленных навыков и их состояние
foxxycode skills enable <name>     # включить навык
foxxycode skills disable <name>    # выключить навык без удаления
```

Полное описание см. в **[`docs/skills.md`](docs/skills.md)**.

## Интеграция MCP-серверов

Подключайте внешние инструменты через MCP-серверы. Их можно настроить глобально в `config.yaml` или передать для конкретной сессии через ACP-клиент.

Пример добавления GitHub MCP-сервера в конфигурацию:

```yaml
mcp_servers:
  - name: "github"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      - name: "GITHUB_PERSONAL_ACCESS_TOKEN"
        value: "${GITHUB_TOKEN}"
```

Подробнее в [руководстве по интеграции MCP](docs/mcp-integration.md).

## Шлюз мессенджеров

Соберите проект с **`-tags gateway.telegram`** (только Telegram) или **`-tags gateway`** (все адаптеры), чтобы включить команду `foxxycode gateway`.

```bash
make build TAGS="gateway.telegram"
./build/foxxycode gateway --config ~/.foxxycode/config.yaml
```

Минимальное дополнение к `config.yaml`:

```yaml
gateways:
  telegram:
    enabled: true
    token: "${TELEGRAM_BOT_TOKEN}"
    admins: [YOUR_USER_ID]
    default_access: "admins"   # all | admins | group:<name>
    default_isolation: "admin" # individual | shared | admin
    rich_messages: true        # Bot API 10.1 Rich Messages (нативный Markdown и блоки инструментов)
```

Каждый пользователь или чат получает отдельную изолированную сессию. В группах бот отвечает только на упоминания и ответы на свои сообщения. Команда `/clear` без пробела начинает новую сессию.

При `rich_messages: true` бот использует [Rich Messages из Bot API 10.1](https://core.telegram.org/bots/api#rich-messages): весь Markdown агента (заголовки, таблицы, код, списки задач) отображается нативно, активность инструментов транслируется вместо заполнителя «Thinking…», а выполненные инструменты показываются в сворачиваемом блоке. Если сервер Bot API не поддерживает эту возможность, бот возвращается к прежнему форматированию. Подробнее в [docs/gateway.md](docs/gateway.md#rich-messages).

Полное руководство по уровням доступа, режимам изоляции групп, настройкам отдельных чатов и созданию новых адаптеров: **[docs/gateway.md](docs/gateway.md)**.

## Справочник по конфигурации

Полное описание доступно в [docs/config.md](docs/config.md), таблицы отдельных полей — в [docs/config-reference.md](docs/config-reference.md). [JSON Schema](docs/config.schema.json) включает автодополнение и проверку в редакторе через заголовок `# yaml-language-server: $schema=...` (см. `config.example.yaml`).

Основные настройки:

```yaml
providers:
  - name: local
    type: openai
    api_key: "${OPENAI_API_KEY}"
    api_base: "${OPENAI_API_BASE}"

models:
  - model: "local/gpt-4o"
    max_tokens: 8192
    temperature: 0.2

agent:
  model: "local/gpt-4o"
  max_turns: 30

tools:
  require_permission_for_commands: true
```

## Архитектура

```text
ACP-клиент (редактор / скрипт / CI)      Мессенджер (Telegram и другие)
        |                                        |
    JSON-RPC 2.0 через stdio             Шлюз Hub (горутина адаптера)
        |                                        |
    Уровень ACP-сервера                  session.Manager (общий)
        |                                        |
    Менеджер сессий ─────────────────────────────┘
        |
    Цикл ReAct-агента
 /      |       |      \
LLM  Инструменты Навыки  MCP
```

Полное описание см. в [документации по архитектуре](docs/architecture.md).

## Документация

- [Отличия FoxxyCode от coddy-agent](docs/vs-coddy.md) — функции форка в сравнении с исходным проектом
- [Сборка из исходников](docs/build.md) — требования, **`make build`**, отличие **`TAGS`** от **`go build -tags`**, каталог **`build/foxxycode`**
- [Обновление FoxxyCode](docs/update.md) — **`foxxycode update`**, артефакты релизов, **`PATH`** и **`make install`**
- [Docker](docs/docker.md) — образ GHCR, **`docker compose`**, встроенный интерфейс по адресу **`http://127.0.0.1:12345/`**
- [Архитектура](docs/architecture.md) — устройство системы и обзор компонентов
- [Протокол ACP](docs/acp-protocol.md) — справочник по протоколу и форматы сообщений
- [Агент ReAct](docs/react-agent.md) — устройство цикла ReAct и спецификации инструментов
- [Конфигурация](docs/config.md) — полное описание файла конфигурации, [таблицы полей](docs/config-reference.md) и [JSON Schema](docs/config.schema.json) для проверки в редакторе
- [HTTP API](docs/http-api.md) — REST-шлюз (**`-tags=http`**) и встроенный интерфейс (**`-tags=http,ui`**), включая **`/foxxycode/config`** для редактирования YAML в SPA (**#/settings**)
- [Встроенный интерфейс](docs/ui.md) — функциональная спецификация, разработка через Vite и теги сборки
- [DESIGN.md](DESIGN.md) — токены и компоновка интерфейса (на английском языке)
- [AGENTS.md](AGENTS.md) — карта репозитория и памятка для автоматизированных участников
- [Правила](docs/rules.md) — правила проекта (`.cursor/rules`, `.foxxycode/rules` и другие)
- [Навыки](docs/skills.md) — slash-команды и **`skills.dirs`**
- [Интеграция MCP](docs/mcp-integration.md) — руководство по MCP-серверам
- [Шлюз мессенджеров](docs/gateway.md) — адаптер Telegram-бота, изоляция сессий, ACL и создание новых адаптеров

## Примеры (ACP через stdio)

[**`examples/acp/acp_e2e_todo.py`**](examples/acp/acp_e2e_todo.py) — построчная JSON-RPC-среда для **`foxxycode acp`** ( **`stdbuf -oL`**, автоматический ответ на запрос разрешения, ответы с nil-result). Используйте её как основу для минимального клиента, а не объединяйте простые команды **`echo`** в конвейер.

[**`examples/acp/acp_e2e_memory.py`**](examples/acp/acp_e2e_memory.py) запускает **`build/foxxycode`** с изолированным **`FOXXYCODE_HOME`** и **`RPA_API_KEY`**, чтобы проверить чтение, сохранение и опциональную очистку Markdown-файлов в **`$FOXXYCODE_HOME/memory`**. Параметры описаны в docstring скрипта. Обзор всех примеров: [**`examples/README.md`**](examples/README.md).

## Постоянные сессии

По умолчанию `foxxycode acp` и `foxxycode http` сохраняют каждую сессию в **`$FOXXYCODE_HOME/sessions/<sessionId>/`** (обычно **`~/.foxxycode/sessions/`**): там находятся `session.json`, `messages.json`, каталог `assets/`, файл `todos/active.md` и каталог `todos/archive/` для заменённых завершённых списков. Корневой каталог можно изменить через **`foxxycode acp --sessions-dir`**, **`foxxycode http --sessions-dir`** или **`sessions.dir`** в **`config.yaml`**. Если каталог сессий невозможно создать, запуск завершается ошибкой.

- **`foxxycode sessions list`** выводит сохранённые сессии и поддерживает фильтры `--sessions-dir` и `--cwd`.
- **`foxxycode acp --session-id <id>`** заставляет **следующий** вызов `session/new` открыть сохранённое состояние этой папки, если оно существует, либо создать новую сессию с таким именем каталога.
- **`session/load`** восстанавливает историю и уведомляет клиента; **`session/list`** перечисляет сохранённые сессии для ACP-совместимых клиентов.

Инструменты foxxycode_todo_* синхронизируют активный список с `todos/active.md`. Полная замена через **`foxxycode_todo_plan_replace`** при наличии незавершённых пунктов отклоняется: сначала завершите их или выполните **`foxxycode_todo_plan_archive`**. Если все пункты имеют состояние **`completed`**, при замене прежний `active.md` перемещается в **`todos/archive/`** под именем `todo-<nanos>.md`. Команда **`foxxycode_todo_plan_archive`** переводит открытые пункты в состояние **`completed`**, записывает **`todos/archive/plan_<unix_seconds>.md`** и очищает план сессии, если включено постоянное хранение.

Если сохранённый план **не пуст**, агент добавляет в шаблон системного промпта заголовок **`### Current todo checklist`** и строки Markdown-списка. Используются встроенные шаблоны либо файлы из **`prompts.dir`**, заданные через **`prompts.agent_prompt`**, **`prompts.plan_prompt`** и **`prompts.docs_prompt`**; стандартные имена — **`agent.md`**, **`plan.md`** и **`docs.md`**. Вставка выполняется через `{{if .TodoList}}` … `{{end}}` и пропускается, когда список пуст. Перед **каждым** вызовом LLM в рамках одного запроса **`session/prompt`** FoxxyCode обновляет системное сообщение, поэтому созданный или изменённый ранее в том же эпизоде ReAct список сразу остаётся видимым.

## Разработка

```bash
# Запуск тестов
go test ./...
make test

# Примеры среды (см. examples/README.md):
# ./examples/build_foxxycode.sh && ./examples/test_acp.sh && ./examples/test_httpserver.sh

# Полнофункциональная локальная сборка (HTTP + UI + планировщик), как в Docker
make build TAGS="http ui scheduler memory"

./build/foxxycode -v    # то же, что --version

# Запуск с отладочными логами в режиме ACP; доступны --log-output, --log-file, --log-format
foxxycode acp --log-level debug

# Только простая однострочная проверка (ответы могут не содержать JSON-RPC "result" при nil;
# для полноценной проверки используйте examples/acp/acp_e2e_todo.py)
echo '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{}}}' | foxxycode acp
```

## Лицензия

Проект распространяется по лицензии MIT. Полный текст находится в файле [LICENSE](LICENSE) в корне репозитория.
