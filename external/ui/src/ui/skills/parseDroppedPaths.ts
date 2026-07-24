/**
 * Parses a native drag payload into absolute filesystem path candidates. The VS Code
 * Explorer (and OS file drags) put **`text/uri-list`** and/or **`text/plain`** on the
 * **`DataTransfer`**; dragging an **editor tab** additionally (on some builds, only)
 * carries VS Code's own **`ResourceURLs`** type — a JSON array of URI strings. The
 * composer relativizes the result via **`POST /foxxycode/workspace/relativize`**
 * before inserting an **`@`**-mention.
 */

/** Converts a **`file://`** / **`vscode-file://`** URI to a filesystem path, or null. */
function uriToPath(raw: string): string | null {
  const s = raw.trim();
  if (s === "" || s.startsWith("#")) {
    return null;
  }
  const m = /^(?:vscode-file|file):\/\/[^/]*(\/.*)$/i.exec(s);
  if (!m) {
    return null;
  }
  let path: string;
  try {
    path = decodeURIComponent(m[1]!);
  } catch {
    path = m[1]!;
  }
  // Windows drive URIs decode to "/C:/x" — drop the leading slash.
  if (/^\/[a-zA-Z]:\//.test(path)) {
    path = path.slice(1);
  }
  return path;
}

/** True for absolute POSIX (**`/x`**) or Windows (**`C:\x`** / **`C:/x`**) paths. */
function looksAbsolute(s: string): boolean {
  return s.startsWith("/") || /^[a-zA-Z]:[\\/]/.test(s);
}

/** URI strings from a VS Code **`ResourceURLs`** payload (a JSON array), or none. */
function resourceUrlList(raw: string): string[] {
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter((x): x is string => typeof x === "string");
  } catch {
    // Not the payload we expected; the other sources still apply.
    return [];
  }
}

/**
 * Extracts absolute path candidates from a drop payload. **`uriList`** wins, then
 * **`resourceUrls`** (VS Code editor tabs); the **`plain`** text is a last-resort
 * fallback used only when neither yielded a path. Results are de-duplicated in
 * source order.
 */
export function parseDroppedPaths(input: {
  uriList?: string | null;
  resourceUrls?: string | null;
  plain?: string | null;
}): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  const push = (p: string) => {
    const t = p.trim();
    if (t !== "" && !seen.has(t)) {
      seen.add(t);
      out.push(t);
    }
  };

  if (input.uriList) {
    for (const line of input.uriList.split(/\r?\n/)) {
      const p = uriToPath(line);
      if (p) {
        push(p);
      }
    }
  }

  if (input.resourceUrls) {
    for (const uri of resourceUrlList(input.resourceUrls)) {
      const p = uriToPath(uri);
      if (p) {
        push(p);
      }
    }
  }

  if (out.length === 0 && input.plain) {
    for (const line of input.plain.split(/\r?\n/)) {
      const s = line.trim();
      if (s === "" || s.startsWith("#")) {
        continue;
      }
      const asUri = uriToPath(s);
      if (asUri) {
        push(asUri);
      } else if (looksAbsolute(s)) {
        push(s);
      }
    }
  }

  return out;
}
