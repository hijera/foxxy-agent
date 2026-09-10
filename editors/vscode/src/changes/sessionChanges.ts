import { httpGet } from "../util/http";

/** One file in a session change set, as reported by
 *  `GET /foxxycode/sessions/{id}/changes?include=content`. Mirrors the Go
 *  `changedFileDTO` in external/httpserver/foxxycode_changes.go and the
 *  Kotlin `ChangedFile` in FoxxyCodeSessionChangesService.kt. */
export interface ChangedFile {
  path: string;
  status: "added" | "modified" | "deleted" | string;
  additions: number;
  deletions: number;
  binary: boolean;
  before: string;
  after: string;
}

function str(o: Record<string, unknown>, key: string): string {
  const v = o[key];
  return v === undefined || v === null ? "" : String(v);
}

function num(o: Record<string, unknown>, key: string): number {
  const v = Number(o[key]);
  return Number.isFinite(v) ? v : 0;
}

/** Parse a change-set response body. Returns [] for anything unexpected so a
 *  version skew between plugin and backend empties the view instead of throwing
 *  into the tree provider. */
export function parseChangedFiles(body: string): ChangedFile[] {
  try {
    const parsed = JSON.parse(body) as { files?: unknown };
    if (!Array.isArray(parsed.files)) return [];
    return parsed.files.map((raw) => {
      const o = (raw ?? {}) as Record<string, unknown>;
      return {
        path: str(o, "path"),
        status: str(o, "status"),
        additions: num(o, "additions"),
        deletions: num(o, "deletions"),
        binary: o.binary === true,
        before: str(o, "before"),
        after: str(o, "after"),
      };
    });
  } catch {
    return [];
  }
}

/** The `+N −M` suffix shown beside a row, or a marker for a binary file. */
export function statLabel(file: ChangedFile, binaryLabel: string): string {
  if (file.binary) return binaryLabel;
  return `+${file.additions} −${file.deletions}`;
}

function joinUrl(base: string, path: string): string {
  return (base.endsWith("/") ? base : base + "/") + path;
}

/** The session the SPA panel last had open in this project. The panel records
 *  it on every session switch, so the view follows whatever chat the user is
 *  looking at without a bridge into the webview. */
export async function fetchLastSessionId(
  baseUrl: string,
  timeoutMs = 5000,
): Promise<string> {
  const res = await httpGet(joinUrl(baseUrl, "foxxycode/project/last-session"), timeoutMs);
  if (res.status < 200 || res.status >= 300) return "";
  try {
    const body = JSON.parse(res.body) as { session_id?: unknown };
    return typeof body.session_id === "string" ? body.session_id : "";
  } catch {
    return "";
  }
}

/** Every file the session changed, with both sides so the diff opens without a
 *  second round trip per row. */
export async function fetchSessionChanges(
  baseUrl: string,
  sessionId: string,
  timeoutMs = 15000,
): Promise<ChangedFile[]> {
  const path = `foxxycode/sessions/${encodeURIComponent(sessionId)}/changes?include=content`;
  const res = await httpGet(joinUrl(baseUrl, path), timeoutMs);
  if (res.status < 200 || res.status >= 300) return [];
  return parseChangedFiles(res.body);
}
