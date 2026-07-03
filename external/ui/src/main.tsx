import React from "react";
import ReactDOM from "react-dom/client";
import "./styles.css";
import { App } from "./ui/App";
import { bootstrapUiThemeFromCookie } from "./ui/theme/uiTheme";
import { installFoxxyUiApi } from "./ui/theme/foxxyUiApi";

bootstrapUiThemeFromCookie();
installFoxxyUiApi();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
