/**
 * Russian overlay for the config JSON Schema served by the Go backend
 * (`internal/config/ui_schema.go`, `GET /foxxycode/config/schema`). Keys are the
 * exact English `title` / `description` strings from that schema; missing entries
 * fall back to English. `schemaEnumLabelRu` maps enum *tokens* (written to config
 * verbatim) to human-readable Russian labels shown in the dropdowns.
 *
 * The `schemaStrings.test.ts` coverage test walks the committed
 * `__fixtures__/ui-schema.json` snapshot and fails if any title, description, or
 * enum token here is missing — so keep this in sync when the schema changes
 * (regenerate the fixture with `UPDATE_UI_SCHEMA_FIXTURE=1 go test ./internal/config`).
 */

export const schemaTextRu: Record<string, string> = {
  // Root
  "FoxxyCode config": "Конфигурация FoxxyCode",
  "Runtime configuration edited via the Settings UI. Secrets are included in GET responses.":
    "Конфигурация времени выполнения, редактируемая через интерфейс настроек. Секреты включаются в ответы GET.",

  // Providers
  "LLM providers": "Провайдеры LLM",
  "API credentials and transport selection for upstream LLM vendors.":
    "Учётные данные API и выбор транспорта для внешних поставщиков LLM.",
  "Provider name": "Имя провайдера",
  "Logical id used in model ids (provider/model-id). ASCII letters, digits, hyphen, and underscore only; must start with a letter. When api_key is empty, the runtime reads the key from the environment variable NAME_API_KEY (NAME is this field in uppercase with hyphens mapped to underscores).":
    "Логический идентификатор, используемый в id моделей (provider/model-id). Только латинские буквы, цифры, дефис и подчёркивание; должен начинаться с буквы. Если api_key пуст, ключ читается из переменной окружения NAME_API_KEY (NAME — это поле в верхнем регистре, дефисы заменены на подчёркивания).",
  "Provider type": "Тип провайдера",
  "Wire protocol for this provider entry.":
    "Протокол обмена для этой записи провайдера.",
  "API base URL": "Базовый URL API",
  "Optional override of the default API base URL for this provider. Ignored for neuraldeep, which always uses https://api.neuraldeep.ru/v1.":
    "Необязательная замена базового URL API по умолчанию для этого провайдера. Игнорируется для neuraldeep — он всегда использует https://api.neuraldeep.ru/v1.",
  "API key": "API-ключ",
  "You may set a literal key, reference ${ENV} in YAML (expanded when the file is loaded), or leave empty so the process reads the conventional NAME_API_KEY variable derived from the provider name (see provider name description).":
    "Можно задать ключ напрямую, сослаться на ${ENV} в YAML (подставляется при загрузке файла) или оставить пустым — тогда процесс прочитает стандартную переменную NAME_API_KEY, производную от имени провайдера (см. описание имени провайдера).",
  "API key command": "Команда получения API-ключа",
  "Optional credential-helper command. When api_key is empty it is run via the shell and its trimmed stdout is used as the key (like git/docker credential helpers or AWS credential_process), letting the provider fetch short-lived or login-issued keys without storing a static secret. On failure resolution falls back to the conventional NAME_API_KEY variable.":
    "Необязательная команда-помощник для получения учётных данных. Если api_key пуст, она запускается через шелл, а её обрезанный stdout используется как ключ (как credential helpers в git/docker или AWS credential_process), позволяя получать короткоживущие или выданные при входе ключи без хранения статического секрета. При ошибке разрешение откатывается к стандартной переменной NAME_API_KEY.",
  "HTTP or SOCKS proxy": "HTTP- или SOCKS-прокси",
  "Optional per-provider outbound proxy. Use http:// or https:// for an HTTP proxy, or socks5:// / socks5h:// for SOCKS5 (socks5h resolves hostnames via the proxy). It overrides any proxy inherited from the environment or the editor. NO_PROXY is still honored and local addresses always connect directly. Leave empty to use the environment/editor proxy (HTTP_PROXY/HTTPS_PROXY), or connect directly when there is none.":
    "Необязательный исходящий прокси для конкретного провайдера. Используйте http:// или https:// для HTTP-прокси либо socks5:// / socks5h:// для SOCKS5 (socks5h разрешает имена хостов через прокси). Он переопределяет прокси, унаследованный из окружения или из настроек редактора. NO_PROXY по-прежнему учитывается, а локальные адреса всегда подключаются напрямую. Оставьте пустым, чтобы использовать прокси окружения/редактора (HTTP_PROXY/HTTPS_PROXY), либо прямое соединение, если его нет.",

  // Models
  "Logical models": "Логические модели",
  "Named model entries the agent and UI can select; ids reference provider prefixes.":
    "Именованные записи моделей, доступные для выбора агенту и интерфейсу; id ссылаются на префиксы провайдеров.",
  "Model id": "Идентификатор модели",
  "Logical id in the form provider/api-model-id; must match a provider name prefix.":
    "Логический id в виде provider/api-model-id; должен совпадать с префиксом имени провайдера.",
  "Max tokens": "Макс. токенов",
  "Upper bound on completion tokens the model may emit for one assistant message.":
    "Верхняя граница токенов ответа, которые модель может выдать в одном сообщении ассистента.",
  Temperature: "Температура",
  "Sampling temperature for this logical model (0 = deterministic, higher = more random).":
    "Температура сэмплирования для этой логической модели (0 = детерминированно, выше = более случайно).",
  "Max context tokens (UI hint)": "Макс. токенов контекста (подсказка UI)",
  "Optional UI hint for composer context bar; 0 means derive from provider metadata when available.":
    "Необязательная подсказка UI для полосы контекста композера; 0 означает вывести из метаданных провайдера, если доступно.",
  Multimodal: "Мультимодальность",
  "When true, the model accepts image or file inputs in addition to text. The UI will offer file attachment for messages sent with this model.":
    "Если включено, модель принимает изображения или файлы в дополнение к тексту. Интерфейс предложит прикрепление файлов для сообщений, отправляемых этой моделью.",
  "Reasoning levels": "Уровни рассуждения",
  "Optional override of the reasoning levels offered for this model (e.g. low, medium, high). Leave empty to auto-detect from the model id; an explicit empty list hides the reasoning selector.":
    "Необязательная замена уровней рассуждения для этой модели (например, low, medium, high). Оставьте пустым для автоопределения по id модели; явно пустой список скрывает выбор уровня рассуждения.",
  "Default reasoning level": "Уровень рассуждения по умолчанию",
  "Reasoning level pre-selected for new chats with this model. Must be one of the resolved reasoning levels; ignored otherwise.":
    "Уровень рассуждения, выбранный по умолчанию для новых чатов с этой моделью. Должен быть одним из доступных уровней; иначе игнорируется.",

  // MCP servers (+ env / headers)
  "MCP servers": "Серверы MCP",
  "Model Context Protocol servers started or contacted for new sessions.":
    "Серверы Model Context Protocol, запускаемые или используемые для новых сессий.",
  "Server type": "Тип сервера",
  "stdio runs a local command; http connects to a remote MCP endpoint.":
    "stdio запускает локальную команду; http подключается к удалённому MCP-эндпоинту.",
  "Server name": "Имя сервера",
  "Stable id referenced by the agent; must be unique in this list.":
    "Стабильный id, на который ссылается агент; должен быть уникальным в этом списке.",
  Command: "Команда",
  "Executable for stdio transport (leave empty when using http url).":
    "Исполняемый файл для транспорта stdio (оставьте пустым при использовании http url).",
  Arguments: "Аргументы",
  "Argv passed after command for stdio MCP servers.":
    "Аргументы (argv), передаваемые после команды для stdio-серверов MCP.",
  Environment: "Окружение",
  "Extra environment variables for the stdio child process.":
    "Дополнительные переменные окружения для дочернего stdio-процесса.",
  "Variable name": "Имя переменной",
  "Environment variable name passed to the MCP process.":
    "Имя переменной окружения, передаваемой процессу MCP.",
  Value: "Значение",
  "Variable value.": "Значение переменной.",
  "MCP URL": "URL MCP",
  "HTTP(S) endpoint when type selects an HTTP-based MCP server.":
    "HTTP(S)-эндпоинт, когда выбран тип HTTP-сервера MCP.",
  "HTTP headers": "HTTP-заголовки",
  "Optional headers sent with MCP HTTP requests.":
    "Необязательные заголовки, отправляемые с HTTP-запросами MCP.",
  "Header name": "Имя заголовка",
  "HTTP header name for MCP HTTP transports.":
    "Имя HTTP-заголовка для HTTP-транспортов MCP.",
  "Header value": "Значение заголовка",
  "HTTP header value.": "Значение HTTP-заголовка.",

  // Agent
  "ReAct agent": "Агент ReAct",
  "Defaults for the main agent loop (model id and safety caps).":
    "Настройки по умолчанию для основного цикла агента (id модели и предохранительные лимиты).",
  "Default model": "Модель по умолчанию",
  "Logical model id from the models list used when the client omits a model.":
    "Логический id модели из списка моделей, используемый, когда клиент не указывает модель.",
  "Max turns": "Макс. шагов",
  "Hard cap on ReAct iterations (LLM calls plus tool rounds) for one user request.":
    "Жёсткий лимит итераций ReAct (вызовы LLM плюс раунды инструментов) на один запрос пользователя.",
  "Max tokens per turn": "Макс. токенов за шаг",
  "Upper bound on total tokens (prompt + completion) the model may use in one agent step.":
    "Верхняя граница суммарных токенов (промпт + ответ), которые модель может использовать за один шаг агента.",
  "LLM retry max": "Макс. повторов LLM",
  "Retries after retryable LLM errors such as HTTP 429 before failing the turn.":
    "Число повторов после устранимых ошибок LLM (например, HTTP 429) до провала шага.",
  "LLM retry base ms": "Базовая задержка повтора LLM (мс)",
  "Initial backoff between LLM retries in milliseconds.":
    "Начальная задержка между повторами LLM в миллисекундах.",
  "LLM min interval ms": "Мин. интервал LLM (мс)",
  "Minimum gap between consecutive LLM calls in milliseconds (0 disables pacing).":
    "Минимальный промежуток между последовательными вызовами LLM в миллисекундах (0 отключает ограничение).",

  // Tools
  "Tools and permissions": "Инструменты и разрешения",
  "Filesystem and shell policy for built-in tools.":
    "Политика файловой системы и шелла для встроенных инструментов.",
  "Permission mode": "Режим разрешений",
  'Controls when the agent asks for user approval before running tools. "ask": approve commands and writes. "accept_edits": auto-approve writes, approve commands. "bypass": skip all prompts.':
    "Определяет, когда агент запрашивает подтверждение перед запуском инструментов. «ask»: подтверждать команды и запись. «accept_edits»: автоматически подтверждать запись, спрашивать про команды. «bypass»: пропускать все запросы.",
  "Command allowlist": "Белый список команд",
  "If non-empty, only these shell command prefixes may run without extra policy.":
    "Если список не пуст, без дополнительной политики могут выполняться только эти префиксы команд шелла.",

  // Skills
  Skills: "Навыки",
  "Slash commands and skill packs discovered from these directories.":
    "Слэш-команды и наборы навыков, обнаруживаемые в этих каталогах.",
  "Skill directories": "Каталоги навыков",
  "Search paths for skills. Defaults: ~/.agents/skills (global, shared with npx skills / npx skillsbd), ${FOXXYCODE_HOME}/skills (foxxycode-specific), ${CWD}/.foxxycode/skills (project-local). ${FOXXYCODE_HOME} and ${CWD} expand at runtime.":
    "Пути поиска навыков. По умолчанию: ~/.agents/skills (глобально, общий с npx skills / npx skillsbd), ${FOXXYCODE_HOME}/skills (для foxxycode), ${CWD}/.foxxycode/skills (в проекте). ${FOXXYCODE_HOME} и ${CWD} подставляются во время выполнения.",

  // Memory
  "Long-term memory": "Долговременная память",
  "Optional memory copilot (requires memory build tag and provider).":
    "Необязательный копилот памяти (требует сборки с тегом memory и провайдера).",
  "Turns on the memory copilot for eligible builds.":
    "Включает копилот памяти для поддерживаемых сборок.",
  "Memory model": "Модель памяти",
  "Logical model override for memory LLM calls; empty uses agent model.":
    "Замена логической модели для вызовов LLM памяти; пусто — используется модель агента.",
  "Memory root": "Корень памяти",
  "Filesystem root for memory markdown; empty uses ${FOXXYCODE_HOME}/memory.":
    "Корневой каталог файловой системы для markdown памяти; пусто — ${FOXXYCODE_HOME}/memory.",
  "Recall max turns": "Макс. шагов извлечения",
  "Bounds recall-side LLM rounds in the memory loop.":
    "Ограничивает раунды LLM на стороне извлечения в цикле памяти.",
  "Persist max turns": "Макс. шагов сохранения",
  "Bounds persist-side LLM rounds in the memory loop.":
    "Ограничивает раунды LLM на стороне сохранения в цикле памяти.",
  "Copilot max tokens": "Макс. токенов копилота",
  "Completion token cap for memory copilot calls.":
    "Лимит токенов ответа для вызовов копилота памяти.",
  "Max search hits": "Макс. результатов поиска",
  "Maximum snippets returned by memory search tools.":
    "Максимальное число фрагментов, возвращаемых инструментами поиска по памяти.",

  // Compaction
  "Automatic context compaction": "Автоматическое сжатие контекста",
  "Summarize older turns when the conversation approaches the model context window.":
    "Сжимает старые шаги диалога в сводку, когда контекст приближается к пределу окна модели.",
  "Turns on auto-compaction; only fires near the context window.":
    "Включает авто-сжатие; срабатывает только у предела окна контекста.",
  "Compaction model": "Модель сжатия",
  "Model override for the summary pass; empty uses agent model.":
    "Замена модели для прохода сводки; пусто — используется модель агента.",
  "Threshold percent": "Порог, %",
  "Trigger at this percent of usable context (max_context_tokens - max_tokens); 50..99.":
    "Срабатывает при этом проценте полезного контекста (max_context_tokens - max_tokens); 50..99.",
  "Keep last turns": "Сохранять последних шагов",
  "Most recent user turns preserved verbatim.":
    "Сколько последних шагов пользователя сохраняется без изменений.",
  "Summary max tokens": "Макс. токенов сводки",
  "Completion token cap for the summary generation.":
    "Лимит токенов ответа для генерации сводки.",

  // Title
  "Automatic session title": "Автоматический заголовок сессии",
  "Generate a short LLM thread title after the first exchange in a fresh, non-pinned session.":
    "Генерирует короткий заголовок диалога после первого обмена в новой, не закреплённой сессии.",
  "Turns on backend auto-title generation for all clients.":
    "Включает генерацию заголовка на бэкенде для всех клиентов.",
  "Title model": "Модель заголовка",
  "Model override for the title pass; empty uses agent model. A small, cheap model is a good choice.":
    "Замена модели для прохода заголовка; пусто — используется модель агента. Хорошо подойдёт маленькая дешёвая модель.",
  "Title max tokens": "Макс. токенов заголовка",
  "Completion token cap for the title generation.":
    "Лимит токенов ответа для генерации заголовка.",

  // Scheduler
  Scheduler: "Планировщик",
  "Cron-style scheduled jobs (requires scheduler build tag).":
    "Задачи по расписанию в стиле cron (требует сборки с тегом scheduler).",
  "When true, this process may run the scheduler daemon and REST.":
    "Если включено, этот процесс может запускать демон планировщика и REST.",
  "Jobs directory": "Каталог задач",
  "Directory of job markdown definitions.":
    "Каталог с markdown-описаниями задач.",
  "Max queue": "Макс. очередь",
  "Maximum concurrent scheduled agent runs.":
    "Максимальное число одновременных запусков агента по расписанию.",
  "Job timeout": "Таймаут задачи",
  "Per-job wall-clock limit, e.g. 30m or 1h30m.":
    "Ограничение реального времени на задачу, например 30m или 1h30m.",
  "Retain sessions": "Хранить сессии",
  "How many completed scheduler session folders to keep per job id.":
    "Сколько папок завершённых сессий планировщика хранить на каждый id задачи.",

  // Prompts
  Prompts: "Промпты",
  "Built-in system prompt files relative to dir.":
    "Встроенные файлы системных промптов относительно каталога.",
  "Prompts directory": "Каталог промптов",
  "Optional override directory for prompt markdown files.":
    "Необязательный каталог-замена для markdown-файлов промптов.",
  "Agent prompt file": "Файл промпта агента",
  "Filename for the main agent system prompt.":
    "Имя файла основного системного промпта агента.",
  "Plan prompt file": "Файл промпта плана",
  "Filename for plan-mode system prompt.":
    "Имя файла системного промпта режима планирования.",
  "Per-provider prompts": "Промпты по провайдеру",
  "Select a system prompt tuned to the active model family (falls back to the shared prompt).":
    "Выбирать системный промпт под семейство активной модели (с откатом на общий промпт).",
  "Use a per-family prompt file (agent.<family>.md) when available.":
    "Использовать файл промпта под семейство (agent.<family>.md), когда он есть.",

  // Instructions
  Instructions: "Инструкции",
  "Files read from the session working directory and appended to the system prompt as project instructions (AGENTS.md-compatible).":
    "Файлы, читаемые из рабочего каталога сессии и добавляемые к системному промпту как инструкции проекта (совместимо с AGENTS.md).",
  "Instruction files": "Файлы инструкций",
  'Filenames relative to session CWD to read as instructions. Defaults to ["AGENTS.md"].':
    'Имена файлов относительно CWD сессии для чтения как инструкций. По умолчанию ["AGENTS.md"].',

  // Logger
  Logger: "Логирование",
  "Process log level, outputs, and rotation.":
    "Уровень логов процесса, выводы и ротация.",
  Level: "Уровень",
  "Minimum severity written to configured outputs.":
    "Минимальная важность, записываемая в настроенные выводы.",
  Outputs: "Выводы",
  "Where log lines are written.": "Куда записываются строки логов.",
  "Log file path": "Путь к файлу лога",
  "Destination file when outputs include file.":
    "Файл назначения, когда в выводах указан file.",
  Format: "Формат",
  "text for human logs; json for structured logs.":
    "text — для человекочитаемых логов; json — для структурированных.",
  Rotation: "Ротация",
  "Size-based rotation when logging to a file.":
    "Ротация по размеру при записи логов в файл.",
  "Max file size (MB)": "Макс. размер файла (МБ)",
  "Rotate after the file reaches this size; 0 uses logger defaults.":
    "Ротировать после достижения файлом этого размера; 0 — значения логгера по умолчанию.",
  "Max files": "Макс. файлов",
  "How many rotated segments to retain; 0 uses logger defaults.":
    "Сколько ротированных сегментов хранить; 0 — значения логгера по умолчанию.",

  // Sessions
  Sessions: "Сессии",
  "Where persisted chat bundles are stored.":
    "Где хранятся сохранённые наборы чатов.",
  "Sessions directory": "Каталог сессий",
  "Override sessions root; empty resolves under FOXXYCODE_HOME.":
    "Замена корня сессий; пусто — разрешается внутри FOXXYCODE_HOME.",

  // Gateways / Telegram
  "Messenger gateways": "Шлюзы мессенджеров",
  "Telegram bot gateway (requires the gateway or gateway.telegram build tag).":
    "Шлюз Telegram-бота (требует сборки с тегом gateway или gateway.telegram).",
  Telegram: "Telegram",
  "Telegram bot adapter settings.": "Настройки адаптера Telegram-бота.",
  Enabled: "Включено",
  "Run the Telegram bot (requires the gateway or gateway.telegram build tag).":
    "Запускать Telegram-бота (требует сборки с тегом gateway или gateway.telegram).",
  "Bot token": "Токен бота",
  "BotFather token. Optional here — leave empty to read it from the TELEGRAM_BOT_TOKEN environment variable (e.g. via .env). Secret: when set it is stored in config.yaml and shown in full.":
    "Токен от BotFather. Здесь необязателен — оставьте пустым, чтобы читать из переменной окружения TELEGRAM_BOT_TOKEN (например, через .env). Секрет: если задан, хранится в config.yaml и показывается полностью.",
  "Rich messages": "Rich-сообщения",
  "Use Bot API 10.1 Rich Messages: the agent's native Markdown renders verbatim, tool activity streams as a Thinking placeholder, and executed tools show in a collapsible block. Falls back to legacy formatting if unsupported.":
    "Использовать Rich Messages из Bot API 10.1: родной Markdown агента отображается как есть, активность инструментов стримится как заглушка «Размышление», а выполненные инструменты показываются в сворачиваемом блоке. При отсутствии поддержки — откат к устаревшему форматированию.",
  Proxy: "Прокси",
  "Optional outbound proxy for Telegram API requests. Use http, https, socks5, or socks5h.":
    "Необязательный исходящий прокси для запросов к Telegram API. Используйте http, https, socks5 или socks5h.",
  Admins: "Администраторы",
  "Telegram user IDs with elevated rights; admins always pass access checks.":
    "ID пользователей Telegram с повышенными правами; администраторы всегда проходят проверки доступа.",
  "Default access": "Доступ по умолчанию",
  "Fallback access level for chats without an override: all, admins, or group:<name>.":
    "Уровень доступа по умолчанию для чатов без переопределения: all, admins или group:<name>.",
  "Default isolation": "Изоляция по умолчанию",
  "Fallback session isolation for group chats.":
    "Изоляция сессий по умолчанию для групповых чатов.",
  "User groups": "Группы пользователей",
  "Named sets of user IDs referenced by access as group:<name>.":
    "Именованные наборы ID пользователей, на которые ссылается доступ как group:<name>.",
  "Group name": "Имя группы",
  "Name referenced by access as group:<name>.":
    "Имя, на которое ссылается доступ как group:<name>.",
  "User IDs": "ID пользователей",
  "Telegram numeric user IDs that belong to this group.":
    "Числовые ID пользователей Telegram, входящих в эту группу.",
  "Per-chat overrides": "Переопределения для чатов",
  "Override isolation and access for specific chats.":
    "Переопределить изоляцию и доступ для конкретных чатов.",
  "Chat ID": "ID чата",
  "Telegram chat id; negative for groups and supergroups.":
    "ID чата Telegram; отрицательный для групп и супергрупп.",
  Isolation: "Изоляция",
  "Per-chat session isolation override.":
    "Переопределение изоляции сессий для чата.",
  Access: "Доступ",
  "Per-chat access override: all, admins, or group:<name>.":
    "Переопределение доступа для чата: all, admins или group:<name>.",

  // UI
  UI: "Интерфейс",
  "Embedded SPA preferences for desktop and HTTP UI.":
    "Настройки встроенного SPA для десктопа и HTTP-интерфейса.",
  "UI language": "Язык интерфейса",
  "UI locale for the embedded SPA. Empty means auto-detect from the system or browser locale.":
    "Локаль интерфейса встроенного SPA. Пусто — автоопределение из системы или локали браузера.",
  "Sending messages": "Отправка сообщений",
  'How the main chat composer submits a message. "enter": Enter sends (Shift/Ctrl+Enter insert a newline). "ctrl_enter": Ctrl/Cmd+Enter sends (Enter inserts a newline). "off": disable keyboard send (Send button only).':
    "Как основное окно чата отправляет сообщение. «enter»: отправка по Enter (Shift/Ctrl+Enter — перенос строки). «ctrl_enter»: отправка по Ctrl/Cmd+Enter (Enter — перенос строки). «off»: отправка с клавиатуры отключена (только кнопкой «Отправить»).",

  // Browser tool
  "Browser tool": "Инструмент браузера",
  "Interactive browser automation tool (requires the browser build tag; drives a local Chrome/Chromium via chromedp).":
    "Интерактивный инструмент автоматизации браузера (требует build-тег browser; управляет локальным Chrome/Chromium через chromedp).",
  "Turns on the interactive browser tools (navigate, click, fill, screenshot, ...) for eligible builds.":
    "Включает интерактивные инструменты браузера (navigate, click, fill, screenshot, ...) для поддерживаемых сборок.",
  Headless: "Без окна (headless)",
  "Run the browser without a visible window. Enabled by default; disable to watch the automated session.":
    "Запускать браузер без видимого окна. Включено по умолчанию; отключите, чтобы наблюдать за сессией.",
  "Browser executable": "Исполняемый файл браузера",
  "Optional path to a specific Chrome/Chromium binary. Empty lets chromedp auto-detect an installed browser.":
    "Необязательный путь к конкретному бинарю Chrome/Chromium. Пусто — chromedp сам найдёт установленный браузер.",
  "Action timeout (seconds)": "Таймаут действия (секунды)",
  "Per-action timeout for navigation, clicks, and other browser operations.":
    "Таймаут на каждое действие: навигацию, клики и прочие операции браузера.",
};

/**
 * Human-readable Russian labels for enum tokens. The token itself remains the
 * value written to config; only the dropdown label changes. Tokens are globally
 * unique across the schema, so a flat token→label map is unambiguous.
 */
export const schemaEnumLabelRu: Record<string, string> = {
  // provider.type
  openai: "OpenAI",
  anthropic: "Anthropic",
  neuraldeep: "NeuralDeep",
  // tools.permission_mode
  ask: "Спрашивать",
  accept_edits: "Авто-подтверждение правок",
  bypass: "Без запросов",
  // isolation
  individual: "Индивидуальная",
  shared: "Общая",
  admin: "Администратор",
  // logger.level
  debug: "Отладка",
  info: "Инфо",
  warn: "Предупреждение",
  error: "Ошибка",
  warning: "Предупреждение (warning)",
  // logger.outputs
  stdout: "stdout",
  stderr: "stderr",
  file: "Файл",
  // logger.format
  text: "Текст",
  json: "JSON",
  // ui.locale
  "": "Авто",
  en: "English",
  ru: "Русский",
  // ui.send_mode
  enter: "Enter",
  ctrl_enter: "Ctrl+Enter",
  off: "Отключено",
};
