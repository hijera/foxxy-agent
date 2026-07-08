import { expect, test } from "vitest";
import { stripFoxxyCodeAttachmentsForUserDisplay, parseSessionAssetFiles } from "./stripFoxxyCodeAttachments";

test("replacing foxxycode_attachment with @path for display", () => {
  const raw =
    `see below\n\n<foxxycode_attachment path="docs/readme.txt" name="readme.txt">\n<![CDATA[hello]]>\n</foxxycode_attachment>`;
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe(
    "see below\n\n@docs/readme.txt",
  );
});

test("decoded XML entities in path attribute", () => {
  const raw = `<foxxycode_attachment path="odd&quot;x.txt" name="x">\n<![CDATA[]]>\n</foxxycode_attachment>`;
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe(`@odd"x.txt`);
});

test("strips foxxycode_session_assets block and preceding newlines", () => {
  const raw =
    "What is in the file?\n\n<foxxycode_session_assets>Uploaded files saved to session assets (read-only). You can read or copy them:\n- /home/user/.foxxycode/sessions/s1/assets/note.txt\n</foxxycode_session_assets>";
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe(
    "What is in the file?",
  );
});

test("strips foxxycode_session_assets when no preceding newline", () => {
  const raw =
    "<foxxycode_session_assets>- /some/path.txt\n</foxxycode_session_assets>";
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe("");
});

test("strips foxxycode_ide_context block and preceding newlines", () => {
  const raw =
    "fix the bug\n\n<foxxycode_ide_context>\n# Active File\nsrc/a.go\n\n# Open Tabs\nsrc/a.go\nsrc/b.go\n</foxxycode_ide_context>";
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe("fix the bug");
});

test("strips foxxycode_terminal_context block and preceding newlines", () => {
  const raw =
    "run the tests\n\n<foxxycode_terminal_context>\n# Active Terminal: zsh\n$ go test ./...\nok\n</foxxycode_terminal_context>";
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe("run the tests");
});

test("strips foxxycode_terminal_output block (with name attr) and preceding newlines", () => {
  const raw =
    'check @terminal\n\n<foxxycode_terminal_output name="zsh">\nfull build log\n</foxxycode_terminal_output>';
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe("check @terminal");
});

test("strips legacy bracket annotation", () => {
  const raw =
    "hello\n\n[Uploaded files saved to session assets (read-only):\n- /path/to/file.txt\nYou can read these files directly or copy them to the workspace as needed.]";
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe("hello");
});

test("parseSessionAssetFiles extracts names from foxxycode_session_assets", () => {
  const content =
    "msg\n\n<foxxycode_session_assets>Uploaded files saved to session assets (read-only). You can read or copy them:\n- /home/user/.foxxycode/sessions/s1/assets/note.txt\n- /home/user/.foxxycode/sessions/s1/assets/doc_1.txt (doc.txt)\n</foxxycode_session_assets>";
  const files = parseSessionAssetFiles(content);
  expect(files).toHaveLength(2);
  expect(files[0].name).toBe("note.txt");
  expect(files[1].name).toBe("doc.txt");
});

test("parseSessionAssetFiles returns empty for content without tag", () => {
  expect(parseSessionAssetFiles("plain message")).toHaveLength(0);
});

test("no duplicate @path when user text already mentioned the attachment", () => {
  const raw =
    `@http_todo_report.md что тут?\n\n` +
    `<foxxycode_attachment path="http_todo_report.md" name="http_todo_report.md">\n` +
    "<![CDATA[# Todo Report]]>\n" +
    `</foxxycode_attachment>`;
  expect(stripFoxxyCodeAttachmentsForUserDisplay(raw)).toBe(
    "@http_todo_report.md что тут?\n\n",
  );
});
