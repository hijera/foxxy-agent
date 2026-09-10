/**
 * The three subagent routes, in one place because two surfaces call them: the
 * Settings tab and the approval notice under a refused `spawn_agent` row.
 *
 * `cwd` is the workspace a receipt is keyed by. It is sent when the caller
 * knows the viewed session's workspace and omitted otherwise, in which case the
 * server answers for the cwd a new session would get.
 */

import type { SubagentCatalog, SubagentCatalogEntry } from "./subagentCatalog";

export type SubagentApiResult<T> = { ok: true; data: T } | { ok: false; error: string };

function withCwd(path: string, cwd?: string | undefined): string {
  const dir = (cwd ?? "").trim();
  return dir ? `${path}?cwd=${encodeURIComponent(dir)}` : path;
}

/** Unwraps the {error:{message}} body these routes answer with. */
async function errorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: { message?: string } };
    return body.error?.message || `HTTP ${res.status}`;
  } catch {
    return `HTTP ${res.status}`;
  }
}

export async function fetchSubagentCatalog(
  cwd?: string | undefined,
): Promise<SubagentApiResult<SubagentCatalog>> {
  let res: Response;
  try {
    res = await fetch(withCwd("/foxxycode/subagents", cwd));
  } catch {
    return { ok: false, error: "network" };
  }
  if (!res.ok) {
    return { ok: false, error: await errorMessage(res) };
  }
  try {
    const body = (await res.json()) as Partial<SubagentCatalog>;
    return {
      ok: true,
      data: {
        items: body.items ?? [],
        workspace: body.workspace ?? "",
        policy: body.policy ?? "ask",
      },
    };
  } catch {
    return { ok: false, error: "parse" };
  }
}

async function postTrust(
  name: string,
  action: "trust" | "untrust",
  cwd?: string | undefined,
): Promise<SubagentApiResult<SubagentCatalogEntry | null>> {
  const dir = (cwd ?? "").trim();
  let res: Response;
  try {
    res = await fetch(`/foxxycode/subagents/${encodeURIComponent(name)}/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(dir ? { cwd: dir } : {}),
    });
  } catch {
    return { ok: false, error: "network" };
  }
  if (!res.ok) {
    return { ok: false, error: await errorMessage(res) };
  }
  try {
    const body = (await res.json()) as { item?: SubagentCatalogEntry };
    return { ok: true, data: body.item ?? null };
  } catch {
    // The receipt was written; only the refreshed row is missing, and the
    // caller reloads the catalog anyway.
    return { ok: true, data: null };
  }
}

export function trustSubagent(
  name: string,
  cwd?: string | undefined,
): Promise<SubagentApiResult<SubagentCatalogEntry | null>> {
  return postTrust(name, "trust", cwd);
}

export function untrustSubagent(
  name: string,
  cwd?: string | undefined,
): Promise<SubagentApiResult<SubagentCatalogEntry | null>> {
  return postTrust(name, "untrust", cwd);
}
