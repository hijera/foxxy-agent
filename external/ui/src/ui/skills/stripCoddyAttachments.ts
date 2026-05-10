import { listAtPathSpans } from "./draftAt";

/** Decodes a minimal subset of XML entities produced by **`encoding/xml.EscapeText`** for attributes. */
function decodeXmlAttrValue(s: string): string {
  return s
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&amp;/g, "&");
}

function workspacePathsEqual(a: string, b: string): boolean {
  return (
    a.replace(/\\/g, "/").trim() === b.replace(/\\/g, "/").trim()
  );
}

/**
 * True when **`text`** already contains a completed **`@path`** workspace mention for **`path`** (same grammar as **`extractAtFileAttachments`**).
 */
function userBubbleAlreadyShowsAtPath(visiblePrefix: string, path: string): boolean {
  if (!path.trim()) {
    return false;
  }
  for (const sp of listAtPathSpans(visiblePrefix)) {
    if (workspacePathsEqual(sp.path, path)) {
      return true;
    }
  }
  return false;
}

/**
 * Collapses persisted **<coddy_attachment>** blocks for transcript UI.
 * Drops the XML (and hides file bodies); inserts **`@path`** only when the user text portion
 * does not already mention that path (**`composer`** previews duplicate **`@`** otherwise).
 */
export function stripCoddyAttachmentsForUserDisplay(raw: string): string {
  const re =
    /<coddy_attachment\b[^>]*\bpath="([^"]*)"[^>]*>[\s\S]*?<\/coddy_attachment\s*>/gi;
  let rebuilt = "";
  let lastIdx = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(raw)) !== null) {
    rebuilt += raw.slice(lastIdx, m.index);
    const pathEnc = m[1] ?? "";
    const path = decodeXmlAttrValue(pathEnc).trim();
    if (path !== "" && userBubbleAlreadyShowsAtPath(rebuilt, path)) {
      rebuilt += "";
    } else if (path !== "") {
      rebuilt += `@${path}`;
    }
    lastIdx = m.index + m[0].length;
  }
  rebuilt += raw.slice(lastIdx);
  return rebuilt;
}
