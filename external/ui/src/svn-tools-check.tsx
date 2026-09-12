// Recorded SVN tool shapes rendered through the production transcript and approval components.
import React from "react";
import { createRoot } from "react-dom/client";
import { ToolCallMessage } from "./ui/messages/ToolCallMessage";
import { PermissionToolPreview } from "./ui/chat/PermissionPromptPreview";
import { buildToolCallPreview } from "./ui/chat/permissionToolPreview";
import { setLocale } from "./ui/i18n/i18n";

const query = new URLSearchParams(location.search);
document.documentElement.dataset.theme =
  query.get("theme") === "light" ? "light" : "dark";
setLocale(query.get("lang") === "ru" ? "ru" : "en");
const calls = [
  {
    name: "info",
    args: {},
    result:
      "branch: branches/encoding\nurl: https://svn.example.test/foxxycode/branches/encoding\nrepository root: https://svn.example.test/foxxycode\nworking copy root: C:\\Projects\\foxxycode\nrevision: 1284",
  },
  {
    name: "status",
    args: {},
    result:
      "M       internal/textenc/decode.go\nA       internal/textenc/decode_test.go\nD       docs/legacy.md\n?       notes.txt\n C      config.yaml\n      C src/renamed.go",
  },
  {
    name: "diff",
    args: { paths: ["internal/textenc/decode.go"] },
    result:
      "Index: internal/textenc/decode.go\n===================================================================\n--- internal/textenc/decode.go\t(revision 1284)\n+++ internal/textenc/decode.go\t(working copy)\n@@ -42,3 +42,3 @@\n func Decode(data []byte) (string, error) {\n-  return string(data), nil\n+  return decoder.Decode(data)\n }",
  },
  {
    name: "commit",
    args: {
      paths: ["internal/textenc/decode.go", "internal/textenc/decode_test.go"],
      message: "fix: preserve Windows-1251 encoding",
    },
    result:
      "Sending        internal/textenc/decode.go\nAdding         internal/textenc/decode_test.go\nTransmitting file data ..done\nCommitted revision 1285.",
  },
  {
    name: "merge",
    args: { source: "branches/release", revision: "1280:1285" },
    result:
      "error: svn: E155015: unresolved conflicts in internal/textenc/decode.go",
  },
  {
    name: "log",
    args: { limit: 3 },
    result:
      "------------------------------------------------------------------------\nr1285 | anton | 2026-09-10 15:42:00 +0300 | 1 line\nChanged paths:\n   M /trunk/internal/textenc/decode.go\n\nfix: preserve Windows-1251 encoding\n------------------------------------------------------------------------",
  },
  {
    name: "list",
    args: { target: "branches" },
    result: "encoding/\nrelease/\nfeature-svn/",
  },
  {
    name: "add",
    args: { paths: ["new file.txt"] },
    result: "A         new file.txt",
  },
  {
    name: "revert",
    args: { paths: ["notes.txt"], recursive: true },
    result: "Reverted 'notes.txt'",
  },
  {
    name: "resolve",
    args: { paths: ["config.yaml"], accept: "working" },
    result: "Resolved conflicted state of 'config.yaml'",
  },
  {
    name: "switch",
    args: { branch: "trunk" },
    result: "Updated to revision 1285.",
  },
  {
    name: "checkout",
    args: { branch: "branches/release", destination: "../release" },
    result: "Checked out revision 1285.",
  },
  { name: "update", args: {}, result: "", status: "in_progress" },
  {
    name: "update",
    args: { paths: ["src"] },
    result: "U    src/main.go",
    status: "cancelled",
  },
];
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
    {calls.map((call, index) => (
      <ToolCallMessage
        key={index}
        toolCallId={`svn-${index}`}
        title={`svn_${call.name}`}
        argsText={JSON.stringify(call.args)}
        resultText={call.result}
        status={call.status || "completed"}
        durationMs={240}
      />
    ))}
    <PermissionToolPreview
      preview={buildToolCallPreview(
        { title: "svn_commit", argsText: JSON.stringify(calls[3]!.args) },
        "",
      )}
    />
  </main>,
);
