// Localized strings for the FoxxyCode VS Code extension.
//
// Mirrors editors/intellij/src/main/resources/messages/FoxxyCodeBundle{,_ru}.properties 1:1.
// Resolution order: explicit `foxxycode.language` setting ("en" / "ru"), or `vscode.env.language`
// when set to "system". Missing keys fall back to English.

export type Locale = "en" | "ru";

const en = {
  // Settings
  "settings.displayName": "FoxxyCode",
  "settings.label.binaryPath": "Binary path (optional):",
  "settings.hint.binaryPath": "Leave empty to use the foxxycode binary bundled with the extension.",
  "settings.label.host": "Host:",
  "settings.label.port": "Port (0 = auto):",
  "settings.label.home": "FoxxyCode home (optional):",
  "settings.label.extraArgs": "Extra args:",
  "settings.checkbox.followTheme": "Match FoxxyCode UI theme to the VS Code color theme",
  "settings.checkbox.nativeDiffs": "Show native inline diffs in the editor when the agent edits files",
  "settings.checkbox.autoApprove": "Auto-apply edits without asking (still shows the diff, with Revert)",
  "settings.language.system": "System",
  "settings.language.en": "English",
  "settings.language.ru": "Русский",

  // First-run
  "firstrun.body": "The foxxycode agent binary is bundled with the FoxxyCode extension and ready to use. The FoxxyCode view in the activity bar will start it automatically. Optional: configure host, port, FoxxyCode home, or override the binary path in Settings.",
  "firstrun.openSettings": "Open Settings",
  "firstrun.dismiss": "Got it",

  // Process lifecycle
  "process.status.starting": "Starting FoxxyCode…",
  "process.status.restarting": "Restarting FoxxyCode…",
  "process.indicator.launching": "FoxxyCode: launching {0}:{1}…",
  "process.error.binaryNotFound": "Bundled foxxycode binary not found. Reinstall the FoxxyCode extension or set a custom binary path in Settings.",
  "process.error.exitedBeforeReady": "FoxxyCode process exited before becoming ready. Verify the binary is a full build and config.yaml has a valid provider API key.",
  "process.error.notReady": "FoxxyCode did not become ready within 30s ({0})",
  "process.error.startFailed": "FoxxyCode failed to start",
  "process.error.startFailedPanel": "FoxxyCode failed to start: {0}",
  "process.button.retry": "Retry",
  "process.button.openSettings": "Open Settings",
  "process.fallback.unavailable": "FoxxyCode is unavailable. Check the binary path and restart.",
  "process.button.openUrl": "Open {0}",

  // Toolbar / commands
  "toolbar.action.restart": "Restart FoxxyCode",
  "toolbar.action.restart.desc": "Restart the FoxxyCode server",
  "toolbar.action.reload": "Reload",
  "toolbar.action.reload.desc": "Reload the page",
  "toolbar.action.openBrowser": "Open in Browser",
  "toolbar.action.openBrowser.desc": "Open the FoxxyCode UI in the system browser",
  "toolbar.action.devtools": "Open DevTools",
  "toolbar.action.devtools.desc": "Open the embedded webview developer tools",
  "toolbar.action.settings": "FoxxyCode Settings",
  "toolbar.action.settings.desc": "Configure FoxxyCode",

  // Notifications
  "notification.title.startFailed": "FoxxyCode failed to start",

  // Binary validation
  "binary.error.notFound": "File not found: {0}",
  "binary.error.executeVersion": "Could not execute: {0} -v",
  "binary.error.executeHelp": "Could not execute: {0} http --help",
  "binary.error.leanBuild": "This is a lean build without HTTP/UI support. Use a full release binary (built with tags: http, ui, scheduler, memory).",
  "binary.ok.fullBuild": "OK — foxxycode {0} (full build)",

  // Native inline diffs
  "diff.notify.proposed.title": "FoxxyCode wants to edit {0}",
  "diff.notify.proposed.content": "Review the inline diff and accept or reject the change.",
  "diff.notify.applied.title": "FoxxyCode edited {0}",
  "diff.notify.applied.content": "The change was applied. You can revert it or open the full diff.",
  "diff.action.accept": "Accept",
  "diff.action.reject": "Reject",
  "diff.action.showDiff": "Show diff",
  "diff.action.revert": "Revert",
  "diff.window.before": "Before",
  "diff.window.after": "After",
};

type MessageKey = keyof typeof en;

const ru: Record<MessageKey, string> = {
  "settings.displayName": "FoxxyCode",
  "settings.label.binaryPath": "Путь к бинарнику (необязательно):",
  "settings.hint.binaryPath": "Оставьте пустым, чтобы использовать foxxycode, встроенный в расширение.",
  "settings.label.host": "Хост:",
  "settings.label.port": "Порт (0 = авто):",
  "settings.label.home": "Домашняя папка FoxxyCode (необязательно):",
  "settings.label.extraArgs": "Доп. аргументы:",
  "settings.checkbox.followTheme": "Подстраивать тему FoxxyCode UI под тему VS Code",
  "settings.checkbox.nativeDiffs": "Показывать нативные inline-диффы в редакторе при правках агента",
  "settings.checkbox.autoApprove": "Применять правки без запроса (дифф всё равно показывается, с возможностью отката)",
  "settings.language.system": "Системный",
  "settings.language.en": "English",
  "settings.language.ru": "Русский",

  "firstrun.body": "Бинарник агента foxxycode встроен в расширение FoxxyCode и готов к работе. Вид FoxxyCode в активити-баре запустит его автоматически. При необходимости настройте хост, порт, домашнюю папку FoxxyCode или путь к бинарнику в настройках.",
  "firstrun.openSettings": "Открыть настройки",
  "firstrun.dismiss": "Понятно",

  "process.status.starting": "Запуск FoxxyCode…",
  "process.status.restarting": "Перезапуск FoxxyCode…",
  "process.indicator.launching": "FoxxyCode: запуск {0}:{1}…",
  "process.error.binaryNotFound": "Встроенный бинарник foxxycode не найден. Переустановите расширение FoxxyCode или укажите путь к бинарнику в настройках.",
  "process.error.exitedBeforeReady": "Процесс FoxxyCode завершился до готовности. Убедитесь, что бинарник полной сборки и в config.yaml указан корректный API-ключ провайдера.",
  "process.error.notReady": "FoxxyCode не стал готов за 30 с ({0})",
  "process.error.startFailed": "Не удалось запустить FoxxyCode",
  "process.error.startFailedPanel": "Не удалось запустить FoxxyCode: {0}",
  "process.button.retry": "Повторить",
  "process.button.openSettings": "Открыть настройки",
  "process.fallback.unavailable": "FoxxyCode недоступен. Проверьте путь к бинарнику и перезапустите.",
  "process.button.openUrl": "Открыть {0}",

  "toolbar.action.restart": "Перезапустить FoxxyCode",
  "toolbar.action.restart.desc": "Перезапустить сервер FoxxyCode",
  "toolbar.action.reload": "Обновить",
  "toolbar.action.reload.desc": "Перезагрузить страницу",
  "toolbar.action.openBrowser": "Открыть в браузере",
  "toolbar.action.openBrowser.desc": "Открыть интерфейс FoxxyCode в системном браузере",
  "toolbar.action.devtools": "Открыть DevTools",
  "toolbar.action.devtools.desc": "Открыть инструменты разработчика webview",
  "toolbar.action.settings": "Настройки FoxxyCode",
  "toolbar.action.settings.desc": "Настроить FoxxyCode",

  "notification.title.startFailed": "Не удалось запустить FoxxyCode",

  "binary.error.notFound": "Файл не найден: {0}",
  "binary.error.executeVersion": "Не удалось выполнить: {0} -v",
  "binary.error.executeHelp": "Не удалось выполнить: {0} http --help",
  "binary.error.leanBuild": "Это lean-сборка без поддержки HTTP/UI. Используйте полный релизный бинарник (с тегами: http, ui, scheduler, memory).",
  "binary.ok.fullBuild": "OK — foxxycode {0} (полная сборка)",

  "diff.notify.proposed.title": "FoxxyCode хочет изменить {0}",
  "diff.notify.proposed.content": "Просмотрите inline-дифф и примите или отклоните изменение.",
  "diff.notify.applied.title": "FoxxyCode изменил {0}",
  "diff.notify.applied.content": "Изменение применено. Можно откатить или открыть полный дифф.",
  "diff.action.accept": "Принять",
  "diff.action.reject": "Отклонить",
  "diff.action.showDiff": "Показать дифф",
  "diff.action.revert": "Откатить",
  "diff.window.before": "До",
  "diff.window.after": "После",
};

const messages: Record<Locale, Record<MessageKey, string>> = { en, ru };

let currentLocale: Locale = "en";

/** Set the active locale used by `t()`. Called from extension activate() and on config change. */
export function setLocale(locale: Locale): void {
  currentLocale = locale;
}

/** Resolve the active locale from the `foxxycode.language` setting value. */
export function localeFromSetting(setting: string, envLanguage: string): Locale {
  if (setting === "en") return "en";
  if (setting === "ru") return "ru";
  // "system" or anything else: follow the host environment language.
  const tag = (envLanguage || "en").toLowerCase();
  return tag.startsWith("ru") ? "ru" : "en";
}

/** SPA locale id passed as `?lang=` ("en" or "ru"). Same mapping as the IntelliJ plugin. */
export function spaLanguageCode(setting: string, envLanguage: string): "en" | "ru" {
  return localeFromSetting(setting, envLanguage);
}

/** Format a message with `{0}`, `{1}`, … placeholders. */
export function t(key: string, ...params: unknown[]): string {
  const table = messages[currentLocale] ?? messages.en;
  const raw = (table as Record<string, string>)[key] ?? (messages.en as Record<string, string>)[key] ?? key;
  if (params.length === 0) return raw;
  return raw.replace(/\{(\d+)\}/g, (_m, idx: string) => {
    const i = Number(idx);
    return i >= 0 && i < params.length ? String(params[i] ?? "") : `{${idx}}`;
  });
}
