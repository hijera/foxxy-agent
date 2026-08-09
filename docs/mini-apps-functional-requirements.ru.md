# Функциональные требования к мини-приложениям FoxxyCode

Статус: clean-room функциональные требования для MVP.

Английская версия:
[mini-apps-functional-requirements.md](mini-apps-functional-requirements.md).

Связанная архитектурная спецификация и описание JSON-языка:
[mini-apps-spec.ru.md](mini-apps-spec.ru.md).

Поведенческие источники исследования:
[NeuralDeskApp PR #115](https://github.com/vakovalskii/NeuralDeskApp/pull/115)
и [issue #106](https://github.com/vakovalskii/NeuralDeskApp/issues/106).

Документ независимо формулирует поведение FoxxyCode. Он не разрешает копирование
исходного кода или текста спецификации NeuralDeskApp.

## 1. Цель

FoxxyCode должен позволять пользователю без навыков разработки превратить
успешно завершенную сессию в редактируемое, протестированное и переносимое
мини-приложение. Оно должно предоставлять сгенерированную форму ввода, исполнять
версионируемый JSON workflow, проверять результат и отображать объявленные
выходы без зависимости от исходной сессии.

MVP должен отдавать приоритет надежному воспроизведению, а не скорости создания
черновика. Длительная дистилляция допустима, если она создает более безопасную и
воспроизводимую программу.

## 2. Участники

| Участник | Ответственность |
|----------|-----------------|
| Автор | Создает приложение из сессии, редактирует, тестирует и выпускает его. |
| Оператор | Передает входы, разрешает действия, отвечает на checkpoints и получает результат. |
| Дистиллятор | Анализирует сессию и создает или исправляет черновик. |
| Интерпретатор | Валидирует и исполняет JSON-программу. |
| Верификатор | Сравнивает replay с критериями успеха и ожидаемым результатом. |
| Администратор | Будущий участник организационного сбора и общих каталогов; в MVP отдельной UI-роли нет. |

Автор и оператор могут быть одним человеком.

## 3. Язык требований

`ДОЛЖЕН`, `НЕ ДОЛЖЕН`, `СЛЕДУЕТ` и `МОЖЕТ` обозначают обязательность
требования. Id требований остаются стабильными ссылками для реализации, тестов
и будущих документов.

## 4. Интерфейсы продукта

### FR-SURFACE-001 — Поддерживаемые интерфейсы

Создание, редактирование, тестирование, релиз, управление каталогом и
интерактивный запуск ДОЛЖНЫ быть доступны в web и desktop FoxxyCode.

### FR-SURFACE-002 — Исключение IDE

Элементы мини-приложений НЕ ДОЛЖНЫ отображаться в IntelliJ, VS Code, ACP или
других IDE-интерфейсах в MVP.

### FR-SURFACE-003 — Действие в сессии

Завершенная сессия ДОЛЖНА предоставлять действие **Создать мини-приложение**.
Оно ДОЛЖНО быть недоступно во время активного turn.
После принятого session result та же операция МОЖЕТ дополнительно показываться
рядом с result summary как **Сохранить как мини-приложение**.

### FR-SURFACE-004 — Точка входа в каталог

Основная web/desktop-навигация ДОЛЖНА содержать раздел **Мини-приложения** со
списком, поиском, импортом, запуском, редактированием, созданием версии,
архивацией/восстановлением, экспортом и историей.

Архивация является состоянием видимости в каталоге и НЕ ДОЛЖНА менять состояние
жизненного цикла `draft`/`released`.

### FR-SURFACE-005 — Build tag `miniapps`

Поддержка мини-приложений ДОЛЖНА компилироваться только при включенном Go build
tag `miniapps`. Tagged-сборка ДОЛЖНА включать интерпретатор, builder executable,
mini-app HTTP registrations и backend capability `miniapps=true`. Сборка без
tag НЕ ДОЛЖНА регистрировать эти команды или routes и ДОЛЖНА объявлять
capability false.

Web/desktop UI controls и routes ДОЛЖНЫ требовать backend capability.
Исключение IDE остается дополнительным условием и НЕ ДОЛЖНО обходиться build
tag.

### FR-SURFACE-006 — Быстрый каталог из сессии

Открытая web/desktop-сессия ДОЛЖНА позволять открыть и закрыть компактный drawer
каталога мини-приложений без ухода из сессии. Drawer и полный каталог ДОЛЖНЫ
использовать одинаковые records, filters, release resolution и archive
visibility.

## 5. Пригодность сессии к дистилляции

### FR-ELIG-001 — Базовая пригодность

FoxxyCode ДОЛЖЕН отказать до первого model call, если:

- сессия пуста;
- turn еще выполняется;
- сессия недоступна для чтения;
- невозможно определить пользовательскую задачу;
- невозможно определить наблюдаемый или ожидаемый результат.

### FR-ELIG-002 — Классификация сценария

Дистиллятор ДОЛЖЕН классифицировать сценарий:

- `deterministic`: воспроизводится через script, API, MCP/skill и file steps;
- `hybrid`: детерминированные шаги плюс ограниченные LLM/agent steps;
- `agent_heavy`: в основном требует динамического рассуждения, но ограничивается
  входами, tools, outputs и success checks;
- `not_distillable`: стабильный контракт задачи и результата создать нельзя.

Классификация и объяснение ДОЛЖНЫ показываться до генерации черновика.

### FR-ELIG-003 — Вся сессия, один сценарий

Анализу доступна вся сессия, но одна задача дистилляции ДОЛЖНА создавать ровно
один сценарий. При обнаружении нескольких независимых задач автор ДОЛЖЕН выбрать
одну.

### FR-ELIG-004 — Подтверждение сценария

До синтеза workflow автор ДОЛЖЕН иметь возможность подтвердить или изменить:

- название задачи;
- описание;
- ожидаемый результат;
- включенный интервал или логическую задачу сессии;
- известные побочные эффекты, которые тест может повторить.

## 6. Pipeline дистилляции

### FR-DISTILL-001 — Асинхронная задача

Дистилляция ДОЛЖНА выполняться как отменяемая асинхронная задача со стабильным
job id, сохраненной фазой, progress events, прошедшим временем и финальным
результатом.

### FR-DISTILL-002 — Поэтапная обработка

Дистиллятор ДОЛЖЕН использовать отдельные ограниченные фазы вместо одного
неограниченного промпта:

1. очистить и нормализовать snapshot исходной сессии;
2. определить задачу, результат, артефакты и успешный путь;
3. разделить константы, входы оператора, секреты и производимые значения;
4. построить типизированную цепочку workflow;
5. заменить лишнее модельное рассуждение детерминированными скриптами или
   вызовами;
6. определить requirements, permissions, outputs, display и success criteria;
7. провалидировать JSON-программу и bundle;
8. выполнить replay на source fixture;
9. проверить replay;
10. исправить и повторить при неуспешной проверке.

Каждая фаза ДОЛЖНА сохранять структурированный результат, чтобы неуспешная
задача могла показать место и причину остановки.

### FR-DISTILL-003 — Минимизация контекста

Полную очищенную сессию СЛЕДУЕТ передавать только фазам, которым она необходима.
Поздние фазы СЛЕДУЕТ снабжать контрактом задачи, структурированными результатами
предыдущих фаз и только релевантными артефактами. Это снижает расход токенов и
риск утечки данных.

### FR-DISTILL-004 — Выделение успешного пути

Дистиллятор ДОЛЖЕН отличать успешные действия от исследований, ошибок, повторных
попыток и брошенных ветвей. Неудачные действия МОГУТ сохраняться только как
явные fallback-знания.

### FR-DISTILL-005 — Детерминизация

Если сессия содержит скрипты, точные команды, API-вызовы или стабильные
последовательности tools, создавшие принятый результат, черновику СЛЕДУЕТ
сохранять их как детерминированные шаги, а не просить агента заново их найти.

### FR-DISTILL-006 — Явные границы агента

Каждая оставшаяся LLM- или agent-операция ДОЛЖНА стать явным ограниченным шагом
с prompt, model capability, allowed tools, максимальным числом turn, output
schema, timeout и success behavior.

### FR-DISTILL-007 — Редактируемый черновик

Успешный синтез ДОЛЖЕН создать изменяемый черновик и открыть редактор.
Автоматическая публикация или release запрещены.

### FR-DISTILL-008 — Отмена

Отмена ДОЛЖНА остановить model calls, agent cycles, scripts, HTTP, MCP и test
replay. Временные сессии дистилляции и test workspaces ДОЛЖНЫ удаляться, если
автор явно не сохранил неуспешный workspace для отладки.

### FR-DISTILL-009 — Восстановление после ошибки

Неуспешная задача ДОЛЖНА сохранить результат последней валидной фазы и
диагностический отчет. Автор ДОЛЖЕН иметь возможность повторить с безопасной
фазы или начать заново из исходной сессии.

### FR-DISTILL-010 — Классификация source artifacts

Каждый candidate artifact из исходной сессии ДОЛЖЕН классифицироваться как
operator input, bundled asset, test fixture, expected-output example либо
discarded evidence. Автор ДОЛЖЕН иметь возможность проверить и изменить
предложенную классификацию до release.

### FR-DISTILL-011 — Source benchmark evidence

При наличии distillation job СЛЕДУЕТ сохранять total source-session duration,
model/API duration и model input/output token counts как приватные authoring
evidence. Эти значения НЕ ДОЛЖНЫ копироваться в portable program как
source-session provenance. Оценки released app ДОЛЖНЫ строиться по его
собственной test/run history.

## 7. Определение входов и генерируемая форма

### FR-INPUT-001 — Классификация значений

Каждое релевантное значение сессии ДОЛЖНО быть классифицировано как:

- постоянная константа;
- вход оператора;
- runtime secret binding;
- выход предыдущего шага;
- требование к окружению/зависимости;
- source-specific данные, которые следует удалить.

### FR-INPUT-002 — Типы входов

Форма ДОЛЖНА поддерживать:

- однострочную строку;
- многострочный текст;
- integer и number;
- boolean;
- enum;
- date и datetime;
- file;
- multiple files;
- directory;
- secret.

### FR-INPUT-003 — Сгенерированные controls

Дистиллятор ДОЛЖЕН выбрать совместимый control: text field, textarea, number
field, checkbox, select, radio group, date/datetime picker, file picker,
multi-file picker, directory picker или secret field.

### FR-INPUT-004 — Валидация

Входы ДОЛЖНЫ поддерживать обязательность, default, enum, диапазон, длину,
pattern, тип файла, extension/media type, лимит количества/размера и
существование пути.

### FR-INPUT-005 — Зависимости входов

Входы ДОЛЖНЫ поддерживать `visible_when`, `enabled_when` и `required_when`.
Зависимости ДОЛЖНЫ образовывать ацикличный граф.

### FR-INPUT-006 — Редактирование автором

Автор ДОЛЖЕН иметь возможность изменить id, title, description, type, control,
default, validation, order, visibility и привязку к workflow визуально и через
raw JSON.

### FR-INPUT-007 — Динамический ввод оператора

Workflow ДОЛЖЕН поддерживать явный operator-input step, варианты которого могут
формироваться из outputs предыдущих шагов.

## 8. Требования к workflow

### FR-WORKFLOW-001 — Канонический JSON

Версионируемая JSON-программа ДОЛЖНА быть единственным каноническим определением
исполнения. Visual editor state должен выводиться из JSON и записываться в него.

### FR-WORKFLOW-002 — Последовательная семантика

Шаги ДОЛЖНЫ выполняться последовательно, кроме явно объявленных `branch`,
`fallback` и вызова точной версии вложенного mini app.

### FR-WORKFLOW-003 — Типы шагов

Интерпретатор MVP ДОЛЖЕН поддерживать:

- operator input;
- deterministic inline или bundled script;
- универсальную JSON-программу `foxxy-vm/1`;
- явно объявленную внешнюю команду;
- ограниченный agent cycle;
- HTTP API call;
- bundled или declared MCP tool call;
- bundled skill call;
- file operation;
- operator confirmation;
- condition/branch;
- вызов точной версии mini app.

### FR-WORKFLOW-004 — Общие настройки

Каждый исполняемый шаг ДОЛЖЕН поддерживать id, title, condition, timeout, retry,
error policy, объявленные inputs/outputs, logging/redaction policy и финальный
status.

### FR-WORKFLOW-005 — Условия

Условия ДОЛЖНЫ использовать ограниченный data expression language. Произвольные
JavaScript, shell, Go, Python или template expressions как условия запрещены.

### FR-WORKFLOW-006 — Retry и fallback

Retry ДОЛЖЕН иметь конечное число попыток и ограниченный backoff. Fallback ДОЛЖЕН
быть явной валидной последовательностью. Каждая попытка отображается в run log.

### FR-WORKFLOW-007 — Передача выходов

Шаги ДОЛЖНЫ публиковать именованные типизированные outputs. Ссылки на
несуществующие, будущие или несовместимые outputs ДОЛЖНЫ вызывать ошибку до
исполнения.

### FR-WORKFLOW-008 — Протокол скриптов

Scripts и commands ДОЛЖНЫ получать аргументы без неявной shell-конкатенации.
Структурированные результаты СЛЕДУЕТ возвращать в JSON и валидировать по output
schema.

### FR-WORKFLOW-009 — Универсальная JSON VM

MVP ДОЛЖЕН предоставлять версионированный JSON-native стековый язык
`foxxy-vm/1` внутри шага `program`. Он ДОЛЖЕН поддерживать функции, локальные
значения, ограниченные циклы и переходы, исключения, JSON-совместимые значения,
арифметику, сравнение, операции со строками/массивами/объектами, проверку схемы
и типизированный return.

До побочных эффектов валидатор ДОЛЖЕН отклонять неизвестные opcodes,
неразрешенные функции/labels, неверные control targets и imports. Выполнение
ДОЛЖНО соблюдать положительные лимиты политики движка для числа инструкций,
wall time, heap, stack depth, call depth и отмены.

### FR-WORKFLOW-010 — Граница JSON VM с хостом

По умолчанию VM ДОЛЖНА быть чистой и детерминированной. Filesystem, process,
network, time, random, model, MCP, skill и operator effects доступны только
через typed import ids, объявленные шагом. Imports ДОЛЖНЫ проходить через те же
permissions, secrets, timeout, redaction и logging, что обычные workflow steps.

VM НЕ ДОЛЖНА предоставлять `eval`, динамическую загрузку opcodes или неявный
доступ к host environment. Ограниченный язык условий ДОЛЖЕН оставаться
отдельным от VM и НЕ ДОЛЖЕН вызывать host imports.

## 9. Зависимости и переносимость

### FR-PORT-001 — Объявление зависимостей

Bundle ДОЛЖЕН объявлять версию интерпретатора, OS/architecture, executable,
language runtimes, portable packages, модели, secret bindings, network access,
skills и MCP components.

### FR-PORT-002 — Содержимое bundle

В bundle МОГУТ входить inline scripts, bundled script files, skills,
распространяемые MCP components, display assets и validation fixtures.

### FR-PORT-003 — Непереносимые компоненты

Компонент, который нельзя включить юридически или технически, ДОЛЖЕН быть
представлен как external host requirement с точным id и compatibility rule.

### FR-PORT-004 — Portable provisioning

Интерпретатор ДОЛЖЕН поддерживать `silent_private` provisioning без install
prompt во время запуска, если:

- указан доверенный HTTPS source;
- указаны точный checksum, artifact identity и размер либо его лимит;
- declarative install action или bundled install script показан на
  release/import review;
- объявлены network/process permissions;
- полномочия точного release уже приняты.

Скрытая установка ДОЛЖНА писать только в app-specific cache и staging directory
на той же файловой системе. Запрещены privilege elevation, системный package
manager, установка service, изменение глобального `PATH`/registry и запись вне
cache. Download, verification и installation ДОЛЖНЫ быть транзакционными и
безопасными при параллельном запуске. Ошибка ДОЛЖНА сохранить предыдущий cache
и завершить preflight без попытки системного fallback.

Та же политика ДОЛЖНА применяться к извлечению embedded runtime и загрузке
объявленной локальной модели. Операторский интерфейс МОЖЕТ показывать общий
статус подготовки runtime; подробная redacted-диагностика ДОЛЖНА записываться в
scoped app run log.

### FR-PORT-005 — Общая семантика интерпретатора

Встроенный FoxxyCode runtime и автономный executable ДОЛЖНЫ использовать один
schema validator, reference resolver, condition evaluator, step semantics,
permission model и result contract.

### FR-PORT-006 — Импорт и экспорт

Автор ДОЛЖЕН иметь возможность экспортировать draft или точный release одним
bundle и импортировать его в совместимую установку. До записи импорт ДОЛЖЕН
проверить безопасность архива, integrity, совместимость схемы и конфликты
id/version.

### FR-PORT-007 — Режим интерпретатора FoxxyCode

FoxxyCode binary, собранный с `miniapps`, ДОЛЖЕН валидировать, инспектировать и
исполнять путь к `miniapp.json`, `.foxxyapp` bundle либо JSON-программу из
stdin. Исполнение ДОЛЖНО работать без tags `http`, `ui` и `desktop`.

Headless execution ДОЛЖЕН принимать полные JSON inputs, раздельно выдавать
machine-readable status/events и result JSON и завершать preflight ошибкой
вместо открытия необъявленного interaction.

### FR-PORT-008 — Сборка единого executable

Tagged builder FoxxyCode ДОЛЖЕН создавать один app-specific executable,
содержащий version-matched интерпретатор, канонический `miniapp.json`, весь
проверенный bundle, integrity manifest и UI assets для выбранного режима.

Реализация v1 на Go ДОЛЖНА собирать version-matched runner source/template с
payload, включенным через `//go:embed`. Builder ДОЛЖЕН использовать разрешенный
совместимый Go toolchain, МОЖЕТ установить checksum-locked portable toolchain
через `silent_private`, НЕ ДОЛЖЕН зависеть от developer checkout и
ДОЛЖЕН инспектировать результат и проверять его embedded digest.
UI runner templates ДОЛЖНЫ содержать prebuilt version-matched SPA assets;
app-specific build НЕ ДОЛЖЕН требовать Node.js или npm.

Компоненты, которым нужны filesystem paths, МОГУТ при первом запуске
проверяться и извлекаться из executable в app-specific runtime cache.
Нераспространяемые и host-only dependencies остаются preflight requirements.

### FR-PORT-009 — Console и UI build modes

Builder ДОЛЖЕН предоставлять режимы `console` и `ui`. Console mode ДОЛЖЕН
сохранять console subsystem целевой ОС и поддерживать TTY prompts и
неинтерактивный JSON input. UI mode ДОЛЖЕН открывать app-only desktop window,
форма и result view которого строятся по embedded JSON и которое не показывает
общий chat UI FoxxyCode.

Первая обязательная UI target — `windows/amd64` с существующей desktop-оболочкой
WebView2 и Go linker option `-H=windowsgui`. WebView2 Runtime ДОЛЖЕН
определяться как явная platform dependency; один файл приложения не означает,
что WebView2 статически слинкован внутрь него.

### FR-PORT-010 — Режимы model binding

Model requirement ДОЛЖЕН иметь стабильный binding id и выбирать:

- `fixed`: одну точную provider identity и точный provider API model id; либо
- `capability`: разрешенный автором выбор по возможностям.

Agent/model step ДОЛЖЕН ссылаться на binding id. Fixed binding НЕ ДОЛЖЕН скрыто
выбирать похожую, более новую, дешевую или доступную локально модель.
Альтернативы ДОЛЖНЫ задаваться явным упорядоченным fallback из binding ids.

### FR-PORT-011 — Identity и переиспользование provider

Интерпретатор ДОЛЖЕН сопоставлять bundled provider с локальной конфигурацией по
canonical effective `base_url`, а не по локальному alias. Нормализация URL
ДОЛЖНА приводить scheme и host к нижнему регистру, host — к ASCII, удалять
стандартный порт, нормализовать dot-segments и удалять один завершающий slash.
Path, включая `/v1`, ДОЛЖЕН оставаться значимым. User information, query и
fragment запрещены.

Совпадение ДОЛЖНО быть точным без prefix, substring, DNS и автоматического
отождествления `localhost` с loopback alias. Для OpenAI и Anthropic ДОЛЖЕН также
совпадать объявленный тип протокола. Объявленный provider adapter также ДОЛЖЕН
совпадать. Интерпретатор использует только credential и proxy bindings
совпавшего локального provider; secret values НЕ ДОЛЖНЫ копироваться в app или
bundle.

### FR-PORT-012 — Разрешение точной модели

После сопоставления provider интерпретатор ДОЛЖЕН требовать точный API model id.
Он ДОЛЖЕН проверить локальную model configuration и затем МОЖЕТ вызвать
документированный model-list endpoint. Требуемые capabilities ДОЛЖНЫ быть
проверены до первого model step. Отсутствие credentials, provider, точной модели
или capabilities ДОЛЖНО завершать preflight redacted-ошибкой.

Интерпретатор НЕ ДОЛЖЕН угадывать по display name. Provider/model fallback
разрешен только как объявленный программой упорядоченный fallback binding.

### FR-PORT-013 — Bootstrap локального provider

`scope: local` ДОЛЖЕН приниматься только для loopback URL или поддерживаемого
local socket; адрес private LAN НЕ ДОЛЖЕН получать local-bootstrap authority.

Для local binding интерпретатор ДОЛЖЕН проверить endpoint и МОЖЕТ запустить
adapter, скачать точную отсутствующую модель либо загрузить ее в память только
при явном объявлении каждой операции. Model download ДОЛЖЕН подчиняться
`silent_private`: checksum/digest или provider identity lock, storage ceiling,
network permission, private cache, redacted logging и запрет privilege
elevation.
Provider-exposed model digest ДОЛЖЕН быть зафиксирован. Уже запущенный совпавший
provider МОЖЕТ сразу использоваться при наличии точной модели, но
silent-private provisioning НЕ ДОЛЖЕН изменять его общий model store.

MVP ДОЛЖЕН поддерживать:

- Ollama: список через `GET /api/tags`, optional pull через `POST /api/pull` и
  вызовы через объявленный OpenAI-compatible `/v1` endpoint;
- LM Studio: список через `GET /api/v1/models`, optional load через
  `POST /api/v1/models/load`, download только при явном разрешении;
- generic compatible adapter, который умеет probe/list, но не start, pull или
  load без отдельно объявленного adapter recipe.

Adapter ДОЛЖЕН выводить native management endpoints только по версионированной
встроенной same-origin схеме. Заданный автором management path НЕ ДОЛЖЕН
конкатенироваться с provider URL. Model pull по `silent_private` ДОЛЖЕН
использовать `storage_scope: app_cache` и app-managed provider store.

После bootstrap интерпретатор ДОЛЖЕН повторно проверить endpoint, точный model id
и capabilities. Если это невозможно до timeout, run ДОЛЖЕН завершиться до
model request.

## 10. Разрешения и безопасность секретов

### FR-SEC-001 — Определение разрешений

Дистиллятор ДОЛЖЕН определить filesystem, process, network, model, MCP/skill,
secret и operator-interaction permissions каждого шага и bundled script.

### FR-SEC-002 — Расхождение разрешений

Валидатор ДОЛЖЕН отклонить workflow, если шаг требует необъявленные полномочия.
Широкое разрешение НЕ ДОЛЖНО скрывать от release review конкретные executable
или network host.

### FR-SEC-003 — Проверка разрешений

Автор ДОЛЖЕН видеть badges и детали permissions в редакторе. Оператор ДОЛЖЕН
видеть эффективные permissions перед первым запуском и при расширении полномочий
в новой версии.

### FR-SEC-004 — Изоляция секретов

Секреты ДОЛЖНЫ поступать через runtime bindings. Они НЕ ДОЛЖНЫ копироваться из
сессии в draft, fixture, bundle, export, run history или display.

### FR-SEC-005 — Изоляция промптов

Секрет НЕ ДОЛЖЕН попадать в LLM/agent prompt, если JSON явно не привязывает его
к шагу, а release review не показывает disclosure. По умолчанию секреты
передаются через environment, stdin или secret HTTP headers.

### FR-SEC-006 — Redaction

Redaction ДОЛЖЕН применяться к:

- source snapshots;
- phase outputs;
- generated JSON;
- scripts и assets;
- process arguments и environment previews;
- model/agent messages;
- logs и errors;
- HTTP headers/bodies;
- MCP arguments/results;
- run history и displayed results.

### FR-SEC-007 — Очистка перед релизом

Релиз ДОЛЖЕН блокироваться при наличии неустраненных secret, private key, session
id, transcript fragment, пользовательских absolute path, необъявленного файла
или полномочия.

## 11. Same-data replay

### FR-TEST-001 — Source fixture

Черновик ДОЛЖЕН иметь локальный, неэкспортируемый fixture, связывающий входы с
теми же данными успешного сценария. Секреты остаются ссылками, а не значениями.

### FR-TEST-002 — Изолированный workspace

Test replay ДОЛЖЕН выполняться в новом изолированном workspace. Входные файлы
копируются или монтируются read-only по permission plan.

### FR-TEST-003 — Побочные эффекты

Network calls, запись вне workspace, внешние команды и другие side effects
ДОЛЖНЫ показываться и подтверждаться до первого теста. Автор ДОЛЖЕН иметь
возможность заменить шаг fixture или mock для verification.

### FR-TEST-004 — Точная ревизия

Test result ДОЛЖЕН содержать точный draft revision, non-secret inputs, resolved
requirements, версию интерпретатора, attempts и artifacts.

### FR-TEST-005 — Выполнение черновика

Draft МОЖЕТ исполняться только через test flow. Обычный каталог для оператора
ДОЛЖЕН предлагать **Запустить** только released-версиям.

## 12. Проверка и исправление

### FR-VERIFY-001 — Порядок проверок

Верификатор ДОЛЖЕН выполнить детерминированные проверки до model-assisted judge:

1. статусы обязательных шагов;
2. schemas и predicates выходов;
3. наличие и integrity артефактов;
4. свойства файлов/содержимого;
5. опциональное модельное семантическое сравнение;
6. опциональное принятие автором.

### FR-VERIFY-002 — Контракт ожидаемого результата

Черновик ДОЛЖЕН содержать редактируемое человеком описание успешного результата.
Оно МОЖЕТ включать schemas, artifact rules, predicates и judge prompt.

### FR-VERIFY-003 — Отчет о расхождениях

Неуспешная проверка ДОЛЖНА создать структурированные discrepancies:

- id проверки;
- ожидаемый результат;
- фактический результат;
- связанный step и artifact;
- severity;
- предлагаемая область workflow для изменения.

### FR-VERIFY-004 — Автоматический refine loop

Дистиллятор ДОЛЖЕН поддерживать настраиваемый replay/verify/refine loop:

- максимум по умолчанию: 3 цикла;
- разрешенный диапазон: 1–10;
- немедленный early exit после прохождения обязательных проверок;
- каждый цикл создает новый draft revision;
- цикл не может скрыто расширять permissions.

### FR-VERIFY-005 — Контекст исправления

Фаза refine СЛЕДУЕТ получать текущий workflow, failed checks, релевантные логи и
выбранные artifacts. Полную исходную сессию не следует передавать повторно без
явного перезапуска scenario analysis.

### FR-VERIFY-006 — Ручное исправление

Автор ДОЛЖЕН иметь возможность выбрать **Исправить расхождения**, изменить draft
вручную или принять неблокирующее замечание. Blocking discrepancy нельзя принять
для релиза.

### FR-VERIFY-007 — Отладочные артефакты

Редактор ДОЛЖЕН предоставлять безопасные links/preview для artifacts и logs.
Открытие ДОЛЖНО соблюдать web/desktop path и permission rules.

## 13. Редактор черновика

### FR-EDIT-001 — Разделы редактора

Редактор ДОЛЖЕН предоставлять:

- обзор;
- inputs и live preview формы;
- workflow outline и step editor;
- requirements;
- permissions и secrets;
- success checks;
- outputs и display;
- bundle files;
- raw JSON;
- test runs;
- release review.

### FR-EDIT-002 — Валидация JSON

Невалидный raw JSON НЕ ДОЛЖЕН заменять последнюю валидную версию. Ошибка ДОЛЖНА
указывать JSON location и, при возможности, связанный визуальный control.

### FR-EDIT-003 — Autosave и ревизия

Валидные изменения СЛЕДУЕТ сохранять после короткого debounce и увеличивать
`draft_revision`. Невалидные изменения МОГУТ оставаться локально до исправления.

### FR-EDIT-004 — Прогресс дистилляции

Во время дистилляции UI ДОЛЖЕН показывать фазу, elapsed time, номер цикла, token
usage при наличии, cancel и последние диагностические события. До появления
редактируемого результата progress UI СЛЕДУЕТ оставаться компактным.

### FR-EDIT-005 — Области workspace дистилляции

На широком экране редактору СЛЕДУЕТ предоставлять три функциональные области:

1. ordered workflow navigation и source evidence;
2. structured draft editor;
3. authoring refinement assistant.

На узком экране те же функции ДОЛЖНЫ оставаться доступны через tabs или drawers.
Точные размеры и визуальный стиль не входят в контракт.

### FR-EDIT-006 — Навигатор шагов и source evidence

Workflow navigator ДОЛЖЕН показывать каждый шаг в порядке выполнения: number,
type badge, title, validation state и expand/collapse control. Authoring/system
context ДОЛЖЕН быть обозначен отдельно и НЕ ДОЛЖЕН становиться неявным runtime
step.

Только во время дистилляции автор ДОЛЖЕН иметь возможность просматривать
read-only summary принятого session result, detected requirements, candidate
artifacts и очищенный collapsible source-session context. Эти evidence НЕ
ДОЛЖНЫ попадать в canonical JSON, export, released bundle или operator run
history.

### FR-EDIT-007 — Прямое редактирование overview

Structured overview ДОЛЖЕН позволять напрямую менять name, description, goal,
input id/label/type/required и validation, acceptance criteria и permission
summaries. Permission badges ДОЛЖНЫ покрывать как минимум network, filesystem,
process/Git, model, MCP и skill authority и открывать полные permission details,
а не заменять их.

### FR-EDIT-008 — Refinement assistant

Authoring assistant ДОЛЖЕН принимать natural-language запросы на добавление,
удаление, перестановку и изменение steps, inputs, prompts, dependencies,
permissions, success checks и result display. Identity текущих authoring
provider/model СЛЕДУЕТ показывать.

Каждый изменяющий приложение ответ assistant ДОЛЖЕН создавать проверяемый patch
к точной `draft_revision`. Автор ДОЛЖЕН принять или отклонить patch. Принятие
ДОЛЖНО создавать новую revision и НЕ ДОЛЖНО публиковать, выпускать, расширять
permissions без review или обходить validation/test gates. Refinement
conversation и source context НЕ ДОЛЖНЫ встраиваться в приложение.

### FR-EDIT-009 — Действия с draft, release и закрытием

Workspace ДОЛЖЕН предоставлять явные действия **Сохранить черновик**,
**Выпустить** и закрыть. Release ДОЛЖЕН проходить обычные gates. При закрытии с
несохраненными локальными невалидными правками автор получает предупреждение;
валидный autosaved draft ДОЛЖЕН сохраниться.

### FR-EDIT-010 — Прямое преобразование сессии

В непустом web- или desktop-чате без активной генерации ДОЛЖНО быть прямое
действие, которое запускает дистилляцию именно выбранной сессии, открывает
workspace мини-приложений и созданный черновик. В IDE embed и сборках без тега
`miniapps` действие ДОЛЖНО отсутствовать.

### FR-EDIT-011 — Генерация ожидаемого результата

Structured editor ДОЛЖЕН принимать текстовые ожидания автора и предоставлять
явное действие генерации через LLM. В canonical JSON ДОЛЖНЫ сохраняться
переиспользуемый ожидаемый результат, критерий приемки, fixed model binding и
исполняемая prompt-проверка успеха. Runtime-проверка ДОЛЖНА возвращать
структурированный verdict и НЕ ДОЛЖНА показывать или сохранять рассуждения
модели.

### FR-EDIT-012 — Выбор логической модели

Над каждым редактируемым черновиком редактор ДОЛЖЕН показывать настроенные
идентификаторы логических моделей. Выбранная модель ДОЛЖНА разрешаться и
сохраняться как точный fixed provider/model binding `primary`. Этот binding
ДОЛЖЕН использоваться всеми agent steps, модельными success checks, генерацией
ожидаемого результата и authoring assistant. Runtime НЕ ДОЛЖЕН скрыто
подставлять другую логическую модель.

### FR-EDIT-013 — Ручное добавление и удаление inputs и steps

Structured editor ДОЛЖЕН предоставлять явные действия добавления и удаления
элементов `inputs[]` и верхнеуровневых шагов `workflow[]`. Для выбранного шага
ДОЛЖНЫ быть доступны id, title, kind и полный редактируемый JSON шага. Удаление
последнего шага workflow ДОЛЖНО блокироваться.

### FR-EDIT-014 — Ограниченные authoring tools

Authoring assistant ДОЛЖЕН изменять in-memory draft только через объявленные
mini-app tools: чтение документа, изменение metadata, добавление/замена и
удаление inputs и steps, замена редактируемого документа. Число model rounds и
operations в одном запросе ДОЛЖНО быть ограничено. Полный результат ДОЛЖЕН
пройти validation до атомарного сохранения. Рассуждения provider и raw tool
payloads НЕ ДОЛЖНЫ возвращаться в UI или встраиваться в мини-приложение.

### FR-EDIT-015 — Полноэкранный workspace

В browser и desktop режимах workspace мини-приложений ДОЛЖЕН занимать всю
доступную поверхность shell, а не открываться как лист фиксированной ширины.
Ниже 1200px три authoring-области МОГУТ перестраиваться, но ДОЛЖНЫ оставаться
доступными без горизонтального overflow страницы.

## 14. Релиз и версии

### FR-RELEASE-001 — Жизненный цикл

В MVP контент имеет только состояния `draft` и `released`. `testing`, `failed`,
`running` и `interrupted` являются состояниями job/run, а не mini app.

### FR-RELEASE-002 — Условия релиза

Для релиза ДОЛЖНЫ выполняться:

- schema/reference validation;
- разрешенные или явно host-provided requirements;
- отсутствие permission mismatch;
- отсутствие blocking sanitization findings;
- passing same-data test текущей draft revision;
- успешные обязательные checks;
- явное подтверждение человека.

### FR-RELEASE-003 — Неизменяемый релиз

Released version ДОЛЖНА быть immutable. Ее редактирование создает или обновляет
draft.

### FR-RELEASE-004 — Повышение версии

Первая версия по умолчанию — `1.0.0`. Следующий release ДОЛЖЕН иметь большую
SemVer. Редактору СЛЕДУЕТ предлагать patch/minor/major по совместимости.

### FR-RELEASE-005 — Проверка релиза

Release review ДОЛЖЕН показывать:

- version и compatibility diff;
- input/output contract diff;
- summary изменения workflow;
- permission diff;
- dependencies и install scripts;
- bundled skills/MCP;
- sanitization report;
- последний test и verification;
- file integrity manifest.

## 15. Runner оператора

### FR-RUN-001 — Выбор released-версии

Каждый обычный run ДОЛЖЕН исполнять точную released version. Каталог МОЖЕТ по
умолчанию выбирать latest, но записывает разрешенную версию.

### FR-RUN-002 — Preflight

До исполнения runner ДОЛЖЕН проверить inputs, requirements, integrity,
permissions, secret bindings и interaction policy.

### FR-RUN-003 — Live execution

Runner ДОЛЖЕН показывать ordered sanitized step progress, duration,
retry/fallback attempts, artifacts и ожидающие operator interactions. Execution
logs ДОЛЖНЫ записываться в выбранную app run directory и НЕ ДОЛЖНЫ
транслироваться inline как agent transcript.

### FR-RUN-004 — Взаимодействие

Только declared operator-input и confirmation steps могут приостанавливать run.
Headless run ДОЛЖЕН завершать preflight ошибкой, если для обязательного
взаимодействия нет answer policy.

### FR-RUN-005 — Отмена

Отмена ДОЛЖНА распространяться на model calls, agent cycles, child processes,
HTTP, MCP, skills, file operations по возможности и nested mini apps.

### FR-RUN-006 — Представление результата

После исполнения runner ДОЛЖЕН проверить success и отобразить настроенные text,
Markdown, JSON, table, file, directory, archive и generated-media outputs.

### FR-RUN-007 — Повтор

Оператор ДОЛЖЕН иметь возможность повторить run с предыдущими non-secret inputs.
Secret fields остаются пустыми или повторно привязываются.

### FR-RUN-008 — Скрытые внутренние данные агента

Console, UI, HTTP и SSE operator streams ДОЛЖНЫ скрывать raw model reasoning,
chain-of-thought, assistant scratch messages, raw tool calls и raw tool
arguments/results. Они МОГУТ показывать только объявленные вопросы и
подтверждения, безопасный lifecycle/step status, явные typed результаты agent
steps, финальные результаты и artifact metadata.

Raw agent reasoning НЕ ДОЛЖЕН сохраняться. Diagnostic tool events МОГУТ
сохраняться только по проверенной политике `none`, `metadata` или `sanitized` с
redaction до записи.

### FR-RUN-009 — Local или global run root

Каждое приложение или build profile ДОЛЖНО выбрать scope логов `local` или
`global`:

- global:
  `$FOXXYCODE_HOME/apps/<app-slug>--<short-id>/runs/<run-id>/`;
- local:
  `<run-workspace>/.foxxycode/apps/<app-slug>--<short-id>/runs/<run-id>/`.

Local run ДОЛЖЕН иметь явный workspace. UI executable без workspace ДОЛЖЕН
запросить его или применить проверенный fallback на global. Каждая run
directory ДОЛЖНА содержать `run.json`, `events.jsonl`, `execution.log`,
`artifacts/` и приватную директорию extraction/cache `runtime/`.

### FR-RUN-010 — Безопасные метрики выполнения

При наличии runner result ДОЛЖЕН показывать total duration, cumulative
model/API duration, input/output token counts и разрешенные non-secret
provider/model binding ids. Отсутствующие provider metrics ДОЛЖНЫ обозначаться
как unavailable, а не оцениваться по несвязанным source-session values.

## 16. История и диагностика

### FR-HISTORY-001 — Сохраняемая история

Каждый run ДОЛЖЕН сохранять mini-app id/version, timestamps, duration, non-secret
inputs, requirements, attempts, approvals, success checks, outputs, artifacts и
terminal status.

### FR-HISTORY-002 — Статусы

Run status ДОЛЖЕН включать `queued`, `preflight`, `running`, `waiting`,
`succeeded`, `failed`, `cancelled` и `interrupted`.

### FR-HISTORY-003 — Логи

Logs ДОЛЖНЫ быть структурированными и ограниченными size/retention policy.
Runtime должен сохранять данные для диагностики шага без secret и полного
неограниченного model context.

### FR-HISTORY-004 — Прерванный запуск

После restart незавершенный run становится `interrupted`. Resume МОЖЕТ
предлагаться только при durable outputs и safe continuation оставшихся шагов.

### FR-HISTORY-005 — Границы диагностики

Run files НЕ ДОЛЖНЫ содержать chain-of-thought, raw reasoning, secrets или
unredacted tool I/O. Tool diagnostics ДОЛЖНЫ быть ограничены, привязаны к шагу
workflow и содержать только разрешенные metadata либо sanitized/truncated
payload.

### FR-HISTORY-006 — Метрики использования

Run history СЛЕДУЕТ сохранять для run и, при наличии, для отдельных steps total
duration, model/API duration, input/output tokens и разрешенные provider/model
binding ids. Эти metrics ДОЛЖНЫ оставаться non-secret, отличать measured от
unavailable и НЕ ДОЛЖНЫ раскрывать prompts, responses, credentials или raw model
internals.

## 17. Каталог и пути повторного использования

### FR-CATALOG-001 — Поиск и фильтры

Каталог ДОЛЖЕН поддерживать text search и filters по lifecycle, archive
visibility, author, tags и compatibility с платформой.

### FR-CATALOG-002 — Длительность

Каталогу СЛЕДУЕТ показывать last и median successful duration при достаточной
истории.

### FR-CATALOG-003 — Три пути под контролем пользователя

UI ДОЛЖЕН позволять:

1. запустить подходящий release;
2. создать новую версию существующего mini app;
3. проигнорировать mini apps и продолжить обычную agent session.

MVP НЕ ДОЛЖЕН заявлять об автоматической оценке или выборе пути.

### FR-CATALOG-004 — Изменение версии

Изменение released mini app ДОЛЖНО создавать draft будущей большей версии.
Временная мутация release запрещена.

### FR-CATALOG-005 — Компактные карточки и безопасный запуск

Quick drawer ДОЛЖЕН предоставлять поиск по name/tags и явный control
**Показывать архивные**. Карточка released app ДОЛЖНА показывать name, short
description, точную release version, число declared inputs,
compatibility/availability и measured duration, когда она известна. Ей СЛЕДУЕТ
показывать icon или сгенерированную отметку media type.

Каждая карточка ДОЛЖНА предоставлять основное действие **Запустить** и overflow
menu для edit/new version, history, export и archive/restore. **Запустить**
ДОЛЖНО открывать сгенерированную форму точной разрешенной версии и НЕ ДОЛЖНО
начинать побочные эффекты до input validation и preflight.

## 18. HTTP и standalone interfaces

### FR-API-001 — API дистилляции

HTTP surface ДОЛЖЕН поддерживать start, read, stream, scenario confirmation,
retry и cancel задачи дистилляции.

### FR-API-002 — API каталога

HTTP surface ДОЛЖЕН поддерживать list/search, draft CRUD, bundle file CRUD,
validation, sanitization, release, import, export, archive и restore.

### FR-API-003 — API запусков

HTTP surface ДОЛЖЕН поддерживать test/released runs, read/stream run state,
ответы на input/confirmation, cancel и artifact download.

### FR-API-004 — Standalone CLI

С tag `miniapps` FoxxyCode ДОЛЖЕН предоставлять `miniapps validate`, `miniapps
inspect`, `miniapps requirements`, `miniapps run` и `miniapps build`. `run`
ДОЛЖЕН принимать JSON file, bundle или программу из stdin вместе с JSON inputs.
`build` ДОЛЖЕН принимать `--mode console|ui`, target, output и local/global
log-scope.

Headless execution ДОЛЖЕН выдавать machine-readable events и result JSON по
разным output channels. Binary без tag ДОЛЖЕН считать команду отсутствующей, а
не отключенной runtime-функцией.

### FR-API-005 — Документация API

Реализованный HTTP contract ДОЛЖЕН быть представлен в served OpenAPI и
репозиторной HTTP API documentation.

### FR-API-006 — UI capability

HTTP-enabled build ДОЛЖЕН возвращать backend capability, однозначно
показывающую, был ли `miniapps` скомпилирован. SPA ДОЛЖНА использовать это
значение для регистрации или скрытия mini-app navigation и actions.

## 19. Решения clean-room адаптации

Наблюдаемое поведение адаптируется к требованиям FoxxyCode следующим образом:

| Наблюдаемая концепция | Требование FoxxyCode |
|-----------------------|----------------------|
| Многоэтапная дистилляция | Использовать ограниченные структурированные фазы. |
| Replay/verify/refine loop | Использовать 1–10 циклов, early exit и immutable test reports. |
| Agent-assisted replay | Заменить неявное поведение явными JSON agent steps. |
| Workflow хранится как local app/skill | Заменить переносимым JSON bundle и автономным интерпретатором. |
| Project и global copies | Использовать явно выбранный local или global app/run root; никогда не дублировать и не объединять scopes неявно. |
| Draft/testing/published/archived | Только draft/released; testing — run state, archive — visibility. |
| Полная сессия в промптах | Ограничить полным контекстом ранние очищенные фазы. |
| Runtime/global secret maps | Заменить secret bindings и сквозным redaction. |
| Session-side workflow panel | Предоставить quick drawer на тех же данных, что полный каталог Mini Apps FoxxyCode. |
| Трехобластной редактор дистилляции | Разделить step/source navigation, structured editing и authoring refinement; на узком экране использовать tabs/drawers. |
| Видимые source transcript и session result | Хранить как временные очищенные authoring evidence; не делать состоянием portable app. |
| Chat-based refinement | Создавать проверяемые revision-bound patches; не изменять release и не обходить gates. |
| Result duration/API/token chips | Показывать измеренные authoring/run metrics, не добавляя source metrics в provenance релиза. |
| Python и LLM replay | Обобщить до script, JSON VM, command, agent, API, MCP/skill, file и operator steps. |

## 20. Сценарная приемка MVP

### AC-MINIAPP-001 — Успешная дистилляция

Дана завершенная web/desktop-сессия с принятым результатом. Когда автор создает
мини-приложение, FoxxyCode подтверждает один сценарий, выполняет pipeline и
открывает валидный редактируемый draft.

### AC-MINIAPP-002 — Непригодная сессия

Дана сессия без определяемого результата. При создании FoxxyCode останавливается
до synthesis и объясняет, какого result contract не хватает.

### AC-MINIAPP-003 — Same-data verification

Дан валидный draft и source fixture. При test FoxxyCode выполняет isolated run,
детерминированные checks, затем declared judge и сохраняет draft revision и
discrepancies.

### AC-MINIAPP-004 — Автоматическое исправление

Дана неуспешная verification и оставшиеся cycles. При включенном refinement
FoxxyCode создает новую draft revision и повторяет replay до успеха или лимита.

### AC-MINIAPP-005 — Защита секретов

Даны секреты сессии и оператора. После distillation, test, export и normal run ни
одно значение не присутствует в bundle, fixture, prompt без explicit binding,
log, history или display.

### AC-MINIAPP-006 — Условие релиза

Дан draft без passing test текущей revision. При release FoxxyCode отказывает и
показывает невыполненное условие.

### AC-MINIAPP-007 — Переносимость

Дан exported release. После import на совместимом хосте embedded и standalone
interpreters при одинаковых inputs/bindings дают эквивалентную семантику шагов и
выходов.

### AC-MINIAPP-008 — Отсутствие в IDE

Дан IDE-embedded FoxxyCode UI. При открытии сессии controls создания, каталога,
редактора и runner мини-приложений отсутствуют.

### AC-MINIAPP-009 — Управление build tag

Даны эквивалентные binaries, собранные с `miniapps` и без него. При проверке
CLI, HTTP routes и web/desktop UI только tagged binary показывает команды
интерпретатора/builder, routes, capability, кнопку создания и раздел Mini Apps.

### AC-MINIAPP-010 — Headless-интерпретация JSON

Даны валидная JSON-программа и полные JSON inputs. При запуске tagged FoxxyCode
без HTTP/UI workflow завершается, выводит только machine-readable безопасные
operator events и result JSON и сохраняет run в выбранном app root.

### AC-MINIAPP-011 — Console и UI executables

Дан один released bundle. Когда builder создает `console` и `ui` executables,
оба содержат одинаковые канонические программу и bundle; console build
поддерживает TTY/headless input, а Windows UI build открывает desktop-форму,
сгенерированную из JSON.

### AC-MINIAPP-012 — Скрытые internals и scoped logs

Дано приложение с agent и tool steps. При запуске в console и UI ни один
интерфейс не показывает reasoning или raw tool calls, ни один run file не
содержит chain-of-thought или unredacted tool I/O, а redacted diagnostics
записываются только в выбранный local или global
`.foxxycode/apps/<app>/runs/<run-id>` root.

### AC-MINIAPP-013 — Универсальная JSON-программа

Дан шаг `foxxy-vm/1` с функциями и ограниченным циклом. При запуске embedded и
built interpreters дают одинаковый typed result; неизвестный opcode,
необъявленный host import или исчерпанный лимит завершают объявленный шаг без
получения дополнительных полномочий хоста.

### AC-MINIAPP-014 — Скрытая приватная установка

Дано released app с отсутствующей locked portable dependency. При запуске
точного проверенного release интерпретатор без install prompt проверяет и
транзакционно устанавливает зависимость в app cache. Он не повышает привилегии
и не изменяет системное состояние; ошибка не оставляет активной частичной
версии.

### AC-MINIAPP-015 — Fixed provider и local model

Дан fixed binding, у которого normalized base URL, protocol type и exact model id
совпадают с локальной конфигурацией FoxxyCode. При запуске интерпретатор
использует локальный credential/proxy binding и вызывает именно эту модель. Для
объявленного loopback provider с отсутствующей моделью он выполняет probe,
optional start, pull/load и проверку точной модели по `silent_private` либо
завершает preflight без подстановки другой модели.

### AC-MINIAPP-016 — Workspace дистилляции

Дан редактируемый distilled draft. При открытии workspace показывает ordered
typed steps, принятый session result, очищенный source context, detected
requirements/artifacts, редактируемые goal/inputs/acceptance criteria,
permission summaries и refinement assistant. Export draft или release не
содержит source transcript, source metrics или refinement conversation.

### AC-MINIAPP-017 — Refinement, привязанный к revision

Дан запрос изменить step, input или prompt. Ответ refinement assistant
показывает patch к текущей точной revision. Reject ничего не меняет; accept
создает новую draft revision, которая по-прежнему требует обычных validation,
testing, sanitization и human release.

### AC-MINIAPP-018 — Быстрый каталог из сессии

Дана открытая web/desktop-сессия. После открытия Mini Apps drawer пользователь
может искать по name/tag, включать и исключать archived apps, видеть exact
version/input count/description/availability и открывать overflow actions.
**Запустить** открывает сгенерированную форму и не вызывает workflow side effect
до preflight.

### AC-MINIAPP-019 — Измеренные usage metrics

При наличии provider metric data source evidence, test или released-run view
показывает total duration, model/API duration и input/output tokens как measured.
Отсутствующие значения обозначаются unavailable, а не угадываются;
source-session metrics отсутствуют в released portable program.

## 21. Отложенная функциональность

- автономный сбор сессий с opt-in;
- организационное утверждение и shared catalogs;
- semantic/vector search;
- автоматическая оценка времени трех путей и routing;
- parallel workflow branches;
- signed bundles и publisher trust;
- remote execution workers;
- scheduled mini-app runs;
- совместное редактирование drafts.
