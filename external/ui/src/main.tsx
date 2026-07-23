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
import { bootstrapEmbedFlag } from "./ui/embedShell";
import { installEmbedLocaleBridge } from "./ui/embedLocaleBridge";
import { installRemoteFetchShim } from "./ui/env/remoteEnv";
import { startActiveHealthMonitor } from "./ui/env/activeHealth";

// Install the remote fetch shim before anything renders so every /v1/* and /foxxycode/* request
// is routed to the selected environment (local origin by default).
installRemoteFetchShim();
bootstrapUiThemeFromCookie();
const bootLocale = bootstrapUiLocaleFromUrlOrCookie();
initLocale(bootLocale);
installFoxxyCodeUiApi();
bootstrapDesktopFlag();
bootstrapEmbedFlag();
installEmbedLocaleBridge();
startActiveHealthMonitor();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <I18nProvider>
        <App />
      </I18nProvider>
    </AppErrorBoundary>
  </React.StrictMode>,
);
