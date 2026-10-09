import ReactDOM from "react-dom/client";
import "./styles.css";
import { ToolCallMessage } from "./ui/messages/ToolCallMessage";
import { I18nProvider } from "./ui/i18n/I18nProvider";
import { initLocale } from "./ui/i18n/i18n";
import { isUiLocale } from "./ui/i18n/locales";

const locale = new URLSearchParams(location.search).get("lang") || "en";
initLocale(isUiLocale(locale) ? locale : "en");

ReactDOM.createRoot(document.getElementById("root")!).render(
  <I18nProvider>
    <main style={{ padding: "16px" }}>
      {["qa_mcp__echo", "a_very_long_mcp_server_name__echo", "read"].map(
        (name, index) => (
          <ToolCallMessage
            key={name}
            toolCallId={`narrow-${index}`}
            title={name}
            status="completed"
            argsText={JSON.stringify({
              path: "a/long/workspace/path/that/needs/to/shrink/notes.txt",
            })}
            resultText="ok"
            durationMs={125}
            onFetchToolCallFull={async () => {}}
          />
        ),
      )}
    </main>
  </I18nProvider>,
);
