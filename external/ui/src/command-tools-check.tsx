// Production tool cards with short, wrapped and multiline commands and output.
import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import { ToolCallMessage } from "./ui/messages/ToolCallMessage";
import { PermissionToolPreview } from "./ui/chat/PermissionPromptPreview";
import { buildToolCallPreview } from "./ui/chat/permissionToolPreview";
import { setHostShell } from "./ui/chat/hostShell";
import { setLocale } from "./ui/i18n/i18n";

const query = new URLSearchParams(location.search);
document.documentElement.dataset.theme =
  query.get("theme") === "light" ? "light" : "dark";
setLocale(query.get("lang") === "ru" ? "ru" : "en");
setHostShell("C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe");
const longCommand = Array.from(
  { length: 40 },
  (_, i) => `Write-Output "Command line ${i + 1}"`,
).join("\n");
const fullOutput = Array.from(
  { length: 80 },
  (_, i) => `Output line ${i + 1}: verified command execution`,
).join("\n");
const calls = [
  {
    id: "short",
    command: "git status",
    result: "On branch main\nworking tree clean",
  },
  {
    id: "failed",
    command:
      "Get-Command sh, bash, git-bash, msys2 -ErrorAction SilentlyContinue\n  | Select-Object Name, Source",
    result: "Command failed: exit status 1",
    status: "failed",
  },
  { id: "long", command: longCommand, result: "Command line 40" },
  {
    id: "wrapped",
    command: `Write-Output "${"long_argument_".repeat(50)}"`,
    result: "long_argument_".repeat(50),
  },
  {
    id: "output",
    command: "Get-ChildItem -Recurse",
    result: fullOutput.split("\n").slice(0, 20).join("\n"),
    truncated: true,
  },
  {
    id: "running",
    command: "go test ./...",
    result: "",
    status: "in_progress",
  },
];

function CommandCards() {
  const [full, setFull] = useState("");
  return (
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
      {calls.map((call) => (
        <ToolCallMessage
          key={call.id}
          toolCallId={call.id}
          title="run_command"
          kind="execute"
          argsText={JSON.stringify({
            command: call.command,
            timeout_seconds: 10,
          })}
          resultText={call.result}
          status={call.status || "completed"}
          durationMs={3000}
          resultWasTruncated={call.truncated}
          fullResultText={call.truncated ? full : undefined}
          {...(call.truncated
            ? { onFetchToolCallFull: async () => setFull(fullOutput) }
            : {})}
        />
      ))}
      <PermissionToolPreview
        preview={buildToolCallPreview(
          {
            title: "run_command",
            argsText: JSON.stringify({
              command: longCommand,
              timeout_seconds: 10,
            }),
          },
          "",
        )}
      />
    </main>
  );
}
createRoot(document.getElementById("root")!).render(<CommandCards />);
