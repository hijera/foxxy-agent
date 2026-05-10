import { expect, test } from "vitest";
import { stripCoddyAttachmentsForUserDisplay } from "./stripCoddyAttachments";

test("replacing coddy_attachment with @path for display", () => {
  const raw =
    `see below\n\n<coddy_attachment path="docs/readme.txt" name="readme.txt">\n<![CDATA[hello]]>\n</coddy_attachment>`;
  expect(stripCoddyAttachmentsForUserDisplay(raw)).toBe(
    "see below\n\n@docs/readme.txt",
  );
});

test("decoded XML entities in path attribute", () => {
  const raw = `<coddy_attachment path="odd&quot;x.txt" name="x">\n<![CDATA[]]>\n</coddy_attachment>`;
  expect(stripCoddyAttachmentsForUserDisplay(raw)).toBe(`@odd"x.txt`);
});

test("no duplicate @path when user text already mentioned the attachment", () => {
  const raw =
    `@http_todo_report.md что тут?\n\n` +
    `<coddy_attachment path="http_todo_report.md" name="http_todo_report.md">\n` +
    "<![CDATA[# Todo Report]]>\n" +
    `</coddy_attachment>`;
  expect(stripCoddyAttachmentsForUserDisplay(raw)).toBe(
    "@http_todo_report.md что тут?\n\n",
  );
});
