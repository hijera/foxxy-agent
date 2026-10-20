// Real composer in its empty-chat and docked layouts, with deterministic mention search.
import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import { Composer } from "./ui/chat/Composer";
import { NavRail } from "./ui/nav/NavRail";
import { setLocale } from "./ui/i18n/i18n";
import "./styles.css";

const query = new URLSearchParams(location.search);
const empty = query.get("state") !== "docked";
document.documentElement.dataset.embed = "intellij";
document.documentElement.dataset.effects = "reduced";
document.documentElement.dataset.theme =
  query.get("theme") === "light" ? "light" : "dark";
setLocale("en");
localStorage.clear();

const fetchOriginal = window.fetch;
window.fetch = async (input, init) => {
  const url = new URL(String(input), location.href);
  if (url.pathname === "/foxxycode/mentions") {
    const items = url.searchParams.get("q")?.startsWith("docs/")
      ? [{ kind: "file", insert: "@docs/README.md", label: "docs/README.md" }]
      : [
          ...Array.from({ length: 16 }, (_, i) => ({
            kind: "file",
            insert: `@file-${i}.md`,
            label: `src/main/vueprojects/projects/surety_kom/src/components/LongComponentName${i}.vue`,
          })),
          {
            kind: "directory",
            insert: "@docs/",
            label: "docs/",
            continue: true,
          },
        ];
    return new Response(JSON.stringify({ items, total: items.length }), {
      headers: { "Content-Type": "application/json" },
    });
  }
  if (url.pathname === "/foxxycode/mentions/check")
    return new Response(JSON.stringify({ mentions: [] }), {
      headers: { "Content-Type": "application/json" },
    });
  return fetchOriginal(input, init);
};

function Check() {
  const [draft, setDraft] = useState("");
  const composer = (
    <Composer
      value={draft}
      isEmpty={empty}
      mode="agent"
      modes={["agent"]}
      onModeChange={() => {}}
      onChange={setDraft}
      onSend={() => {}}
    />
  );
  return (
    <div className="shell">
      <NavRail
        historyOpen={false}
        onOpenHistory={() => {}}
        onNewChat={() => {}}
        onOpenSettings={() => {}}
        settingsOpen={false}
        onOpenScheduler={() => {}}
        schedulerOpen={false}
        canWidenRail={false}
        railLabelsWide={false}
        onToggleRailLabels={() => {}}
      />
      <div className="shell-main">
        <main className="main">
          <div className="chat-stack">
            {empty ? (
              <section className="hero">
                <div className="hero-composer">{composer}</div>
              </section>
            ) : (
              <>
                <div id="messages" className="chat-scroll">
                  <div className="messages-inner">Existing turn</div>
                </div>
                <div className="chat-bottom chat-bottom--docked">
                  <div className="chat-bottom-inner">{composer}</div>
                </div>
              </>
            )}
          </div>
        </main>
      </div>
    </div>
  );
}
createRoot(document.getElementById("root")!).render(<Check />);
