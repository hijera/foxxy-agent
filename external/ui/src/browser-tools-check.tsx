// Browser fixture renders the production transcript component with recorded tool shapes.
import React from "react";
import { createRoot } from "react-dom/client";
import { ToolCallMessage } from "./ui/messages/ToolCallMessage";
import { setLocale } from "./ui/i18n/i18n";

const query = new URLSearchParams(location.search);
document.documentElement.dataset.theme =
  query.get("theme") === "light" ? "light" : "dark";
setLocale(query.get("lang") === "ru" ? "ru" : "en");
const result =
  "navigated\nurl: https://example.com/\nscreenshot: browser-fixture.png\npage log:\n  [warn] Slow response\n  [error] GET /api: 500";
const calls = [
  { name: "navigate", args: { url: "https://example.com/" }, result },
  {
    name: "evaluate",
    args: {
      expression: `(()=>{${Array.from({ length: 18 }, (_, i) => `const value${i}=${i};`).join("")}return {title:document.title,count:18};})()`,
    },
    result: 'result: {"title":"Example","count":18}',
  },
  {
    name: "screenshot",
    args: {},
    result: "captured screenshot\nscreenshot: browser-fixture.png",
  },
  {
    name: "click",
    args: { selector: 'form.checkout > button[type="submit"]' },
    result,
  },
  { name: "fill", args: { selector: "#search", text: "FoxxyCode" }, result },
  { name: "hover", args: { selector: ".navigation > a" }, result },
  { name: "scroll", args: { x: -120, y: 400 }, result },
  {
    name: "read_page",
    args: {},
    result:
      'page: Example\nurl: https://example.com/\n  heading "Example"\n  button "Submit" [selector="#submit"]',
  },
  {
    name: "page_log",
    args: {},
    result:
      "page log (2 entries, cleared by this read):\n  [warn] Slow response\n  [error] GET /api: 500",
  },
  {
    name: "inspect",
    args: { what: "storage" },
    result:
      "localStorage (1):\n  theme = dark\nsessionStorage: empty\ncookies: empty\nnote: httpOnly cookies are not visible",
  },
  {
    name: "inspect",
    args: { what: "timing" },
    result:
      "load phases (ms):\n  dns: 2\n  load: 185\nrequests: 3\nslowest:\n  140ms https://example.com/api (210 bytes)",
  },
  {
    name: "inspect",
    args: { what: "memory" },
    result:
      "js_heap: used 4 MB of 8 MB (limit 2 GB)\ndom_nodes: 180\ninline_onclick_handlers: 0",
  },
  { name: "close", args: {}, result: "browser closed" },
];
const selected = query.get("tool");
createRoot(document.getElementById("root")!).render(
  <main
    style={{
      width: "100%",
      minWidth: 0,
      maxWidth: 760,
      boxSizing: "border-box",
      margin: "24px auto",
      padding: 16,
    }}
  >
    {calls
      .filter((call) => !selected || call.name === selected)
      .map((call, index) => (
        <ToolCallMessage
          key={index}
          toolCallId={`browser-${index}`}
          title={`foxxycode_browser_${call.name}`}
          argsText={JSON.stringify(call.args)}
          resultText={call.result}
          status="completed"
          durationMs={240}
          sessionId="fixture"
        />
      ))}
  </main>,
);
