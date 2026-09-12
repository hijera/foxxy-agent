# Встроенный browser-тул foxxy-agent vs Playwright MCP

*Дата: 2026-09-01. Сравнивается `internal/tools/browser` (тулзы `foxxycode_browser_*`, build tag `browser`) с [microsoft/playwright-mcp](https://github.com/microsoft/playwright-mcp) (`@playwright/mcp`) по состоянию на сентябрь 2026.*

## 1. Что это такое

| | foxxycode browser (встроенный) | Playwright MCP |
|---|---|---|
| Форма | Go-пакет внутри бинаря агента; тулзы регистрируются напрямую в реестр | Внешний Node.js-процесс, MCP-сервер (stdio / SSE / Docker) |
| Драйвер | **chromedp** — сырой CDP к локальному Chrome/Chromium | **Playwright** — Chromium, Chrome/Edge (каналы), Firefox, WebKit |
| Кол-во тулз | 11 | ~25 в ядре + ~40 опциональных за `--caps=...` |
| Адресация элементов | **CSS-селекторы** (модель берёт их из `read_page`) | **`ref`-ы из accessibility-снапшота** (+ координаты за `--caps=vision`) |
| Восприятие страницы | Текстовый outline (`read_page`) + **скриншот после каждого действия** (vision-блок модели; отключается `browser.screenshots: false`) | **A11y-снапшот** как основной канал; скриншот — отдельный read-only тул |
| Диагностика страницы | `page_log` — консоль, исключения, упавшая сеть и 4xx/5xx **автоматически в каждом результате действия** | `browser_console_messages`, `browser_network_requests` — по явному запросу |
| Управление доступом | Встроено в permission-систему foxxycode: мутирующие действия (`navigate`, `click`, `fill`, `hover`, `scroll`, `evaluate`) требуют подтверждения, read-only — нет | Сам сервер «не является security boundary»; есть `--allowed-origins` / `--blocked-origins`, маскирование `--secrets`; approval — забота MCP-клиента |
| Жизненный цикл | Один Chrome на сессию агента, ленивый запуск, профиль в `<session>/browser-profile`, per-action таймаут (30s по умолчанию) | Один браузер на сервер; персистентный или `--isolated` профиль, `--storage-state`, `--cdp-endpoint` (чужой браузер), режим расширения Chrome |
| Зависимости | Только установленный Chrome/Chromium (бинарь ищется автоматически, `browser.executable_path` — вручную) | Node.js + npx/Docker + браузеры Playwright |
| Конфиг | YAML `browser:` (enabled, headless, executable_path, timeout_seconds, screenshots) + UI настроек | Десятки CLI-флагов / env / JSON: прокси, эмуляция устройств, init-скрипты, таймауты по типам, вьюпорт, UA и т.д. |

Философская разница: наш тул — **минимальный verify-инструмент агента** («открой dev-сервер, потыкай, посмотри, что сломалось»), заточенный под цикл foxxycode: скриншот сам приходит модели, ошибки страницы сами приходят в результат, permissions встроены. Playwright MCP — **универсальный браузерный комбайн** для любого MCP-агента, с упором на детерминизм (a11y-снапшот вместо картинок) и на тестовую инфраструктуру.

## 2. Инвентарь: наши 11 тулз

| Тул | Что делает | Permission |
|---|---|---|
| `foxxycode_browser_navigate` | Открыть http/https URL (localhost разрешён) | да |
| `foxxycode_browser_click` | Клик по первому видимому элементу по CSS-селектору | да |
| `foxxycode_browser_fill` | `SetValue` в input/textarea (замена значения целиком) | да |
| `foxxycode_browser_hover` | Наведение мыши по селектору | да |
| `foxxycode_browser_scroll` | Скролл к селектору или на x/y пикселей | да |
| `foxxycode_browser_evaluate` | JS-выражение, результат как JSON | да |
| `foxxycode_browser_screenshot` | Свежий скриншот вьюпорта | нет |
| `foxxycode_browser_read_page` | Текстовый outline: роль / имя / value / CSS-селектор, до 400 строк, `interactive_only` | нет |
| `foxxycode_browser_page_log` | Слить накопленный лог: console, uncaught exceptions, network fail/4xx/5xx | нет |
| `foxxycode_browser_inspect` | `storage` (localStorage/sessionStorage/cookies), `timing` (фазы загрузки + медленные запросы), `memory` (JS-heap, DOM-узлы) | нет |
| `foxxycode_browser_close` | Закрыть Chrome сессии | нет |

Детали реализации, влияющие на сравнение:

- `read_page` строит outline **скриптом в странице**, а не через CDP Accessibility domain — `GetFullAXTree` падает на pinned `cdproto` (Chrome шлёт enum-значения, которых нет в схеме). Это наш аналог a11y-снапшота, но самодельный: роли выводятся эвристикой из тегов/`role=`, селектор — из `id`/`name`/`data-testid`/nth-of-type-цепочки.
- `fill` делает `chromedp.SetValue` — значение подменяется, но **клавиатурные события не отправляются** (нет keydown/keyup, нет Enter). React-формы обычно ловят `input`-событие, но автокомплиты/маски/submit-по-Enter — нет.
- Скриншот после каждого мутирующего действия сохраняется в assets сессии и инжектится модели vision-блоком (`env.AddToolImage`), т.е. модель «видит» результат без отдельного запроса — но каждый шаг стоит ~40 КБ base64 (отключаемо).

## 3. Чего нам не хватает по сравнению с Playwright MCP

> **Статус 2026-09-01:** пункты **1–13** реализованы в `internal/tools/browser`
> (тулзы `press_key`, `wait_for`, `tabs`, `select_option`, `handle_dialog`,
> `navigate_back`, `upload_file`, `resize`, `drag`, `fill_form`, `find`,
> `network`+`route`, `storage_set`, плюс клики по координатам в `click`).
> Актуальный список — в [browser-tool.md](browser-tool.md). Ниже — исходный
> анализ, оставлен как обоснование.

Сгруппировано по важности для нашего основного сценария (агент проверяет своё изменение в SPA/dev-сервере).

### Критично — дыры, в которые агент реально упирается

1. **Клавиатура** (`browser_press_key`, type с посимвольным вводом). У нас нельзя нажать Enter/Escape/Tab/стрелки. Формы с submit-по-Enter, модалки, закрывающиеся по Escape, автокомплиты — недостижимы без костыля через `evaluate` + `dispatchEvent`.
2. **Ожидания** (`browser_wait_for` — появление/исчезновение текста, таймаут). У нас только неявные `WaitVisible` внутри действий; ждать «пока прогрузится SPA после клика» агент вынужден циклом `read_page`/`screenshot`, сжигая токены.
3. **Вкладки** (`browser_tabs`: list/new/select/close). У нас одна страница на сессию; ссылка с `target=_blank` уводит контент туда, куда мы не умеем смотреть.
4. **`<select>`** (`browser_select_option`, у browser-use — `dropdown_options`). Наш `fill` через SetValue может попасть в option по value, но перечислить опции и выбрать по label нечем.
5. **Диалоги** (`browser_handle_dialog`). `alert()`/`confirm()`/`beforeunload` блокируют страницу — у нас нет способа их принять/отклонить, действие просто зависнет до таймаута.
6. **Назад/вперёд** (`browser_navigate_back`). История недоступна — только повторный `navigate` на полный URL.

### Важно — расширяет класс проверяемых сценариев

7. **Загрузка файлов** (`browser_file_upload`) — file input непроверяем.
8. **Ресайз вьюпорта / эмуляция устройств** (`browser_resize`, `--device`, `--mobile`). Наш же собственный регламент `ui-verification.md` требует проверки narrow 390px / wide 1280px — встроенным тулом это сделать нельзя (только браузер-пейн Claude Code умеет).
9. **Drag-and-drop** (`browser_drag`/`browser_drop`) — kanban, сортировки, наш же drag-drop файлов в композер.
10. **Мультиполе за один вызов** (`browser_fill_form`) — у нас N round-trip'ов и N скриншотов на форму из N полей.
11. **Поиск по снапшоту** (`browser_find`) — у нас только полный `read_page` (до 400 строк) без фильтра по тексту.
12. **Сеть**: детальный список запросов (`browser_network_requests` + тело ответа), **мок ответов** (`browser_route`), offline-режим. Наш `page_log` видит только сбои и статусы ≥400; успешный, но неправильный ответ API — невидим без `evaluate`.
13. **Запись storage**: у нас `inspect what=storage` только читает; выставить cookie/localStorage — вручную через `evaluate`. У Playwright MCP — полный CRUD + сохранение/загрузка storage state (готовые «залогиненные» состояния для тестов).

### Другой класс задач — не порт, а осознанный выбор

14. **Кросс-браузерность**: Firefox/WebKit/каналы Edge. chromedp = только Chromium, это ограничение архитектуры.
15. **Тестовая обвязка** (`--caps=testing`, `--caps=devtools`): ассерты видимости/значений, генерация локаторов, **codegen** (запись действий в Playwright-код), трейсинг, запись видео с главами, подсветка элементов, пошаговое исполнение. Для сценария «агент чинит e2e-тесты» Playwright MCP вне конкуренции.
16. **PDF** (`browser_pdf_save`).
17. **Координатные клики** (`--caps=vision`, `browser_mouse_*_xy`) — канвасы, карты, кастомные виджеты без DOM-семантики. У нас кликов по координатам нет вовсе.
18. **Подключение к внешнему браузеру**: `--cdp-endpoint`, режим расширения (работа в живом залогиненном Chrome пользователя). Мы всегда запускаем свой процесс.
19. **Сетевые/санитарные рычаги**: прокси, `--allowed-origins`/`--blocked-origins`, `--ignore-https-errors`, `--secrets` (маскирование секретов в снапшотах), init-скрипты, `--test-id-attribute`.

## 4. Что есть у нас и чего нет у Playwright MCP

1. **Ноль внешней инфраструктуры.** Один Go-бинарь + любой установленный Chrome. Не нужен Node/npx, не нужно качать браузеры Playwright (~400 МБ), нет второго процесса-сервера и MCP-хендшейка. Для Docker-сборки и IntelliJ/VS Code-плагинов это существенно.
2. **Замкнутый цикл «действие → картинка → лог».** Каждое мутирующее действие само возвращает URL, скриншот (как vision-вход модели, с сохранением в assets сессии) и page log. У Playwright MCP агент должен сам запросить снапшот/консоль после действия. Наш вариант — меньше ходов на шаг в happy path.
3. **Автосбор ошибок страницы.** Console + uncaught exceptions + network fail/4xx/5xx копятся слушателем CDP с момента запуска и приходят в результат первого же действия. «Белая страница из-за упавшего бандла» видна сразу, без отдельной команды.
4. **Готовые диагностические срезы** — `inspect what=timing` (Navigation Timing + топ-10 медленных ресурсов) и `what=memory` (heap, DOM-узлы). В Playwright MCP этого нет как тулзы — только руками через `evaluate` или полноценный трейсинг.
5. **Text-only режим** (`browser.screenshots: false`) — осознанная поддержка моделей без vision: тулзы прямо описывают, что читать вместо картинки (`read_page`, page log, evaluate).
6. **Интеграция с permission-системой foxxycode** — мутирующие действия проходят через тот же механизм подтверждения, что shell/файлы; у MCP-варианта гранулярность определяется клиентом (обычно «разрешить весь сервер»).
7. **Сессионность из коробки**: браузер и профиль привязаны к сессии агента (`Manager` per session id), артефакты падают в session dir. У Playwright MCP один браузер на сервер (для мультиклиента — `--shared-browser-context` или изолированные контексты, но это уже конфигурация).

## 5. Выводы и рекомендации

**Разница в одну фразу:** наш тул — узкий, самодостаточный и «проактивный» (сам показывает и сам жалуется), Playwright MCP — широкий, инфраструктурный и «реактивный» (всё есть, но каждый факт агент запрашивает явно, зато детерминированно и в любом движке).

Для базового verify-цикла foxxycode наш тул закрывает, по грубой оценке, ~70% потребностей, и его сильные стороны (нулевые зависимости, автоскриншоты в vision, автолог ошибок, permissions) терять не хочется. Разумная стратегия — не замена, а доращивание + опциональное соседство:

1. **Портировать в `internal/tools/browser` первую шестёрку**: `press_key`, `wait_for`, вкладки, `select_option`, `handle_dialog`, `navigate_back`. Всё это делается на chromedp без новых зависимостей и снимает самые частые тупики агента. Следующим эшелоном — `file_upload`, `resize` (нужен для narrow/wide проверок из `ui-verification.md`), `fill_form`.
2. **Не порождать свой Playwright.** Codegen, трейсинг, видео, ассерты, Firefox/WebKit — это чужая ниша. Когда такой сценарий нужен, подключать готовый `@playwright/mcp` как обычный MCP-сервер через `internal/mcp` (trust gate уже есть): пользователь прописывает его в `mcp.json`, и оба набора тулз сосуществуют. Цена — Node.js на машине и второй браузерный стек; поэтому именно опция, а не default.
3. **Точечные заимствования дизайна**: у Playwright MCP стоит забрать идею капабилити-гейтов (наши сетевые/storage-write тулзы, если появятся, лучше прятать за конфиг-флагом, как это сделано с `--caps`), маскирование секретов в выводе `inspect what=storage` (сейчас значения только усечены до 60 символов, но не маскированы) и `ref`-подобную стабильную адресацию из `read_page` вместо хрупких nth-of-type-цепочек.

## Источники

- `internal/tools/browser/*.go`, `internal/config/browser.go` — реализация встроенного тула (этот репозиторий).
- https://github.com/microsoft/playwright-mcp — README и справка по тулзам/капабилити/конфигурации `@playwright/mcp`.
- `docs/architecture.md`, память проекта: browser tool port (Cline `browser_action` → foxxycode, chromedp v0.13.6, build tag `browser`).
