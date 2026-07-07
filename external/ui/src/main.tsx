import React from "react";
import ReactDOM from "react-dom/client";
import "./styles.css";
import { App } from "./ui/App";
import { AppErrorBoundary } from "./ui/AppErrorBoundary";
import { bootstrapUiThemeFromCookie } from "./ui/theme/uiTheme";
import { installFoxxyCodeUiApi } from "./ui/theme/foxxycodeUiApi";
import { bootstrapUiLocaleFromUrlOrCookie } from "./ui/i18n/uiLocale";
import { initLocale } from "./ui/i18n/i18n";
import { I18nProvider } from "./ui/i18n/I18nProvider";
import { bootstrapDesktopFlag } from "./ui/desktopShell";

bootstrapUiThemeFromCookie();
const bootLocale = bootstrapUiLocaleFromUrlOrCookie();
initLocale(bootLocale);
installFoxxyCodeUiApi();
bootstrapDesktopFlag();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <I18nProvider>
        <App />
      </I18nProvider>
    </AppErrorBoundary>
  </React.StrictMode>,
);
