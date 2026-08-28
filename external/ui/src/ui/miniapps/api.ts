import type {
  MiniAppApiResult,
  MiniAppCatalogEntry,
  MiniAppDistillation,
  MiniAppDocument,
  MiniAppAssistantMessage,
  MiniAppAssistantResponse,
  MiniAppPatch,
  MiniAppRun,
  MiniAppRunEvent,
} from "./types";

async function errorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as {
      error?: { message?: string } | string;
      message?: string;
    };
    const nested =
      typeof body.error === "object" && body.error
        ? body.error.message
        : body.error;
    if (typeof nested === "string" && nested.trim()) return nested.trim();
    if (typeof body.message === "string" && body.message.trim())
      return body.message.trim();
  } catch {
    // Fall through to the status line.
  }
  return `HTTP ${res.status}`;
}

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<MiniAppApiResult<T>> {
  try {
    const res = await fetch(path, init);
    if (!res.ok)
      return {
        ok: false,
        status: res.status,
        message: await errorMessage(res),
      };
    if (res.status === 204)
      return { ok: true, status: res.status, data: {} as T };
    return { ok: true, status: res.status, data: (await res.json()) as T };
  } catch {
    return { ok: false, status: 0, message: "network" };
  }
}

function appPath(id: string): string {
  return `/foxxycode/miniapps/${encodeURIComponent(id)}`;
}

function unwrap<T extends Record<string, unknown>>(body: T): T {
  const value = body.data;
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as T)
    : body;
}

export async function listMiniApps(): Promise<
  MiniAppApiResult<MiniAppCatalogEntry[]>
> {
  const result = await request<{
    apps?: MiniAppCatalogEntry[];
    items?: MiniAppCatalogEntry[];
  }>("/foxxycode/miniapps");
  if (!result.ok) return result;
  const body = unwrap(result.data);
  return { ...result, data: body.apps ?? body.items ?? [] };
}

export async function getMiniApp(
  id: string,
): Promise<MiniAppApiResult<MiniAppDocument>> {
  const result = await request<MiniAppDocument>(appPath(id));
  return result.ok
    ? {
        ...result,
        data: unwrap(
          result.data as MiniAppDocument & Record<string, unknown>,
        ) as MiniAppDocument,
      }
    : result;
}

export async function getMiniAppDraft(
  id: string,
): Promise<MiniAppApiResult<MiniAppDocument>> {
  const result = await request<MiniAppDocument>(`${appPath(id)}/draft`);
  return result.ok
    ? {
        ...result,
        data: unwrap(
          result.data as MiniAppDocument & Record<string, unknown>,
        ) as MiniAppDocument,
      }
    : result;
}

export async function getMiniAppRelease(
  id: string,
  version: string,
): Promise<MiniAppApiResult<MiniAppDocument>> {
  const result = await request<MiniAppDocument>(
    `${appPath(id)}/versions/${encodeURIComponent(version)}`,
  );
  return result.ok
    ? {
        ...result,
        data: unwrap(
          result.data as MiniAppDocument & Record<string, unknown>,
        ) as MiniAppDocument,
      }
    : result;
}

export async function getAuthoringSource(
  id: string,
): Promise<MiniAppApiResult<Record<string, unknown>>> {
  return request(`${appPath(id)}/authoring/source`);
}

export async function startDistillation(
  sessionId: string,
  title?: string,
): Promise<MiniAppApiResult<MiniAppDistillation>> {
  const body = title?.trim() ? { title: title.trim() } : {};
  return request(
    `/foxxycode/sessions/${encodeURIComponent(sessionId)}/miniapps/distill`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-FoxxyCode-Session-ID": sessionId,
      },
      body: JSON.stringify(body),
    },
  );
}

export async function getDistillation(
  jobId: string,
): Promise<MiniAppApiResult<MiniAppDistillation>> {
  return request(
    `/foxxycode/miniapp-distillations/${encodeURIComponent(jobId)}`,
  );
}

export async function confirmScenario(
  jobId: string,
  scenario: unknown,
): Promise<MiniAppApiResult<MiniAppDistillation>> {
  return request(
    `/foxxycode/miniapp-distillations/${encodeURIComponent(jobId)}/scenario`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ scenario }),
    },
  );
}

export async function cancelDistillation(
  jobId: string,
): Promise<MiniAppApiResult<MiniAppDistillation>> {
  return request(
    `/foxxycode/miniapp-distillations/${encodeURIComponent(jobId)}/cancel`,
    { method: "POST" },
  );
}

export async function updateDraft(
  id: string,
  draft: MiniAppDocument,
  revision?: string,
): Promise<MiniAppApiResult<MiniAppDocument>> {
  return request(`${appPath(id)}/draft`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      ...(revision ? { "If-Match": revision } : {}),
    },
    body: JSON.stringify({ ...draft, revision: revision ?? draft.revision }),
  });
}

export async function assistMiniApp(
  id: string,
  draft: MiniAppDocument,
  history: MiniAppAssistantMessage[],
  message: string,
): Promise<MiniAppApiResult<MiniAppAssistantResponse>> {
  return request(`${appPath(id)}/assistant`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message, history, draft }),
  });
}

export async function validateDraft(
  id: string,
): Promise<MiniAppApiResult<Record<string, unknown>>> {
  return request(`${appPath(id)}/validate`, { method: "POST" });
}

export async function sanitizeDraft(
  id: string,
): Promise<MiniAppApiResult<Record<string, unknown>>> {
  return request(`${appPath(id)}/sanitize`, { method: "POST" });
}

export async function releaseDraft(
  id: string,
  version: string,
  approved: boolean,
  expectedRevision?: string,
): Promise<MiniAppApiResult<MiniAppDocument>> {
  return request(`${appPath(id)}/release`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      version,
      approved,
      ...(expectedRevision ? { expected_revision: expectedRevision } : {}),
    }),
  });
}

export async function createTestRun(
  id: string,
  inputs: Record<string, unknown>,
): Promise<MiniAppApiResult<MiniAppRun>> {
  return request(`${appPath(id)}/test-runs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ inputs }),
  });
}

export async function createReleaseRun(
  id: string,
  version: string,
  inputs: Record<string, unknown>,
): Promise<MiniAppApiResult<MiniAppRun>> {
  return request(
    `${appPath(id)}/versions/${encodeURIComponent(version)}/runs`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ inputs }),
    },
  );
}

export async function getRun(
  runId: string,
): Promise<MiniAppApiResult<MiniAppRun>> {
  return request(`/foxxycode/miniapp-runs/${encodeURIComponent(runId)}`);
}

export async function confirmRun(
  runId: string,
  approved: boolean,
  confirmationId?: string,
): Promise<MiniAppApiResult<MiniAppRun>> {
  return request(
    `/foxxycode/miniapp-runs/${encodeURIComponent(runId)}/confirmation`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        approved,
        ...(confirmationId ? { confirmation_id: confirmationId } : {}),
      }),
    },
  );
}

export async function cancelRun(
  runId: string,
): Promise<MiniAppApiResult<MiniAppRun>> {
  return request(
    `/foxxycode/miniapp-runs/${encodeURIComponent(runId)}/cancel`,
    { method: "POST" },
  );
}

export async function listRuns(
  id: string,
): Promise<MiniAppApiResult<MiniAppRun[]>> {
  const result = await request<{ runs?: MiniAppRun[]; items?: MiniAppRun[] }>(
    `${appPath(id)}/runs`,
  );
  if (!result.ok) return result;
  const body = unwrap(result.data);
  return { ...result, data: body.runs ?? body.items ?? [] };
}

export async function acceptRepairPatch(
  id: string,
  patchId: string,
): Promise<MiniAppApiResult<MiniAppDocument>> {
  return request(
    `${appPath(id)}/authoring/patches/${encodeURIComponent(patchId)}/accept`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ approved: true }),
    },
  );
}

export async function createRepairProposals(
  id: string,
  report: unknown,
): Promise<
  MiniAppApiResult<{
    patches?: MiniAppPatch[];
    items?: MiniAppPatch[];
  }>
> {
  return request(`${appPath(id)}/authoring/patches`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ report }),
  });
}

export function subscribeMiniAppEvents(
  kind: "distillation" | "run",
  id: string,
  onEvent: (event: MiniAppRunEvent) => void,
): () => void {
  const prefix = kind === "run" ? "miniapp-runs" : "miniapp-distillations";
  const source = new EventSource(
    `/foxxycode/${prefix}/${encodeURIComponent(id)}/events`,
  );
  source.onmessage = (event) => {
    try {
      onEvent(JSON.parse(event.data) as MiniAppRunEvent);
    } catch {
      // Ignore malformed progress frames; the polling fallback still updates the UI.
    }
  };
  return () => source.close();
}

export type MiniAppPatchResult = MiniAppApiResult<MiniAppPatch>;
