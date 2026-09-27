/**
 * Proxy URL helpers for the settings field that hides the proxy password.
 *
 * The field shows the URL with every password character replaced by a dot, one
 * for one, so a position in what the user sees is the same position in the real
 * value: that is what lets an edit made on the masked text be carried over to
 * the real one (applyMaskedEdit).
 *
 * The password is taken to run from the first colon after "scheme://" up to the
 * last "@" - the same split the Go backend makes, which is why a password typed
 * with a raw "@" in it is hidden whole.
 */

export const PROXY_MASK_CHAR = "•";

export const PROXY_SCHEMES = ["http", "https", "socks5", "socks5h"] as const;

export type ProxyScheme = (typeof PROXY_SCHEMES)[number];

export type ProxyParts = {
  scheme: string;
  host: string;
  port: string;
  user: string;
  password: string;
};

function authorityStart(value: string): number {
  const i = value.indexOf("://");
  return i >= 0 ? i + 3 : 0;
}

/**
 * The [start, end) range of the password in a proxy URL, or null when it has
 * none. Before the "@" is typed the text after "user:" is still treated as a
 * password unless it can only be a port, so it is hidden while it is typed.
 */
export function passwordSpan(value: string): [number, number] | null {
  const start = authorityStart(value);
  const at = value.lastIndexOf("@");
  if (at >= start) {
    const colon = value.indexOf(":", start);
    if (colon < 0 || colon > at) {
      return null;
    }
    return [colon + 1, at];
  }
  // No "@" yet: "host:3128" (or "host:3128/") is a port, anything else is a
  // password on its way.
  const colon = value.indexOf(":", start);
  if (colon < 0) {
    return null;
  }
  const tail = value.slice(colon + 1);
  if (/^\d*\/?$/.test(tail)) {
    return null;
  }
  return [colon + 1, value.length];
}

/** The URL as the field shows it: the password dotted out, except `revealAt`. */
export function maskProxyValue(value: string, revealAt?: number | null): string {
  const span = passwordSpan(value);
  if (!span) {
    return value;
  }
  const [start, end] = span;
  let out = value.slice(0, start);
  for (let i = start; i < end; i++) {
    out += i === revealAt ? value[i] : PROXY_MASK_CHAR;
  }
  return out + value.slice(end);
}

export type MaskedEdit = {
  /** The real value after the edit. */
  value: string;
  /** Where the caret belongs afterwards. */
  caret: number;
  /** Index of a single character just typed into the password, else null. */
  typedAt: number | null;
};

/**
 * Carries an edit made on the masked text over to the real value.
 *
 * With the selection before the edit and the caret after it the edit is exact:
 * the text after the caret is unchanged, the text before the old selection is
 * unchanged, and what lies between is what the user typed or pasted (literally -
 * a dot in it maps back to nothing). Without them (a programmatic change) the
 * common prefix and suffix of the two masked strings stand in for them.
 */
export function applyMaskedEdit(
  oldReal: string,
  oldShown: string,
  newShown: string,
  selectionBefore?: [number, number],
  caretAfter?: number,
): MaskedEdit {
  let start = -1;
  let suffix = -1;
  if (selectionBefore && caretAfter !== undefined) {
    const a = Math.min(selectionBefore[0], caretAfter);
    const s = newShown.length - caretAfter;
    // The caret has to agree with the text: what precedes the edit and what
    // follows the caret must be unchanged, and nobody types a dot. A change the
    // caret did not follow (a programmatic one leaves it at the end) fails this,
    // and the diff below is used instead.
    if (
      a >= 0 &&
      s >= 0 &&
      a <= caretAfter &&
      oldShown.length - s >= a &&
      newShown.slice(0, a) === oldShown.slice(0, a) &&
      newShown.slice(caretAfter) === oldShown.slice(oldShown.length - s) &&
      !newShown.slice(a, caretAfter).includes(PROXY_MASK_CHAR)
    ) {
      start = a;
      suffix = s;
    }
  }
  if (start < 0) {
    let p = 0;
    const max = Math.min(oldShown.length, newShown.length);
    while (p < max && oldShown[p] === newShown[p]) {
      p++;
    }
    let s = 0;
    while (
      s < max - p &&
      oldShown[oldShown.length - 1 - s] === newShown[newShown.length - 1 - s]
    ) {
      s++;
    }
    start = p;
    suffix = s;
  }
  const inserted = newShown.slice(start, newShown.length - suffix);
  const value =
    oldReal.slice(0, start) + inserted + oldReal.slice(oldReal.length - suffix);
  let typedAt: number | null = null;
  if (inserted.length === 1 && inserted !== PROXY_MASK_CHAR) {
    const span = passwordSpan(value);
    if (span && start >= span[0] && start < span[1]) {
      typedAt = start;
    }
  }
  return { value, caret: start + inserted.length, typedAt };
}

function decodePart(s: string): string {
  try {
    return decodeURIComponent(s);
  } catch {
    // A "%" that starts no escape is a character the user typed; keep it.
    return s;
  }
}

/** Splits a proxy URL into the editor's fields, decoding the credentials. */
export function parseProxyUrl(value: string): ProxyParts {
  const v = value.trim();
  const parts: ProxyParts = { scheme: "http", host: "", port: "", user: "", password: "" };
  if (!v) {
    return parts;
  }
  let rest = v;
  const sep = v.indexOf("://");
  if (sep >= 0) {
    parts.scheme = v.slice(0, sep).toLowerCase();
    rest = v.slice(sep + 3);
  }
  const at = rest.lastIndexOf("@");
  let hostPort = rest;
  if (at >= 0) {
    const userinfo = rest.slice(0, at);
    hostPort = rest.slice(at + 1);
    const colon = userinfo.indexOf(":");
    if (colon >= 0) {
      parts.user = decodePart(userinfo.slice(0, colon));
      parts.password = decodePart(userinfo.slice(colon + 1));
    } else {
      parts.user = decodePart(userinfo);
    }
  }
  const slash = hostPort.indexOf("/");
  if (slash >= 0) {
    hostPort = hostPort.slice(0, slash);
  }
  if (hostPort.startsWith("[")) {
    const close = hostPort.indexOf("]");
    if (close >= 0) {
      parts.host = hostPort.slice(1, close);
      const after = hostPort.slice(close + 1);
      parts.port = after.startsWith(":") ? after.slice(1) : "";
      return parts;
    }
  }
  const colon = hostPort.lastIndexOf(":");
  if (colon >= 0) {
    parts.host = hostPort.slice(0, colon);
    parts.port = hostPort.slice(colon + 1);
  } else {
    parts.host = hostPort;
  }
  return parts;
}

/**
 * Builds a proxy URL from the editor's fields. The login and password are
 * percent-encoded, so any character - "@", ":", "/", "#", "%", a space, Cyrillic -
 * reaches the proxy as typed. An empty host builds an empty value (no proxy).
 */
export function buildProxyUrl(parts: ProxyParts): string {
  const host = parts.host.trim();
  if (!host) {
    return "";
  }
  const scheme = parts.scheme.trim().toLowerCase() || "http";
  let auth = "";
  if (parts.user !== "" || parts.password !== "") {
    auth = encodeURIComponent(parts.user);
    if (parts.password !== "") {
      auth += ":" + encodeURIComponent(parts.password);
    }
    auth += "@";
  }
  const hostPart = host.includes(":") && !host.startsWith("[") ? `[${host}]` : host;
  const port = parts.port.trim();
  return `${scheme}://${auth}${hostPart}${port ? ":" + port : ""}`;
}

/** Whether a port field holds a usable value: empty, or 1-65535. */
export function validProxyPort(port: string): boolean {
  const p = port.trim();
  if (p === "") {
    return true;
  }
  if (!/^\d+$/.test(p)) {
    return false;
  }
  const n = Number(p);
  return n >= 1 && n <= 65535;
}
