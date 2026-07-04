import React from "react";
import ReactDOM from "react-dom/client";
import "./styles.css";
import { App } from "./ui/App";
import { bootstrapUiThemeFromCookie } from "./ui/theme/uiTheme";
import { installFoxxyCodeUiApi } from "./ui/theme/foxxycodeUiApi";
import { bootstrapUiLocaleFromUrlOrCookie } from "./ui/i18n/uiLocale";
import { initLocale } from "./ui/i18n/i18n";
import { I18nProvider } from "./ui/i18n/I18nProvider";

bootstrapUiThemeFromCookie();
const bootLocale = bootstrapUiLocaleFromUrlOrCookie();
initLocale(bootLocale);
installFoxxyCodeUiApi();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </React.StrictMode>,
);
