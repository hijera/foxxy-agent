import type {
  DistillationJob,
  ExpectedResultGeneration,
  MiniAppAuthoringResult,
  MiniAppAuthoringTurn,
  MiniAppCatalogEntry,
  MiniAppDocument,
  MiniAppRun,
  SourceEvidence,
} from "./types";

type APIResult<T> =
  { ok: true; data: T } | { ok: false; status: number; message: string };

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<APIResult<T>> {
  try {
    const response = await fetch(path, init);
    const text = await response.text();
    let payload: unknown = {};
    if (text.trim()) {
      try {
        payload = JSON.parse(text);
      } catch {
        payload = text;
      }
    }
    if (!response.ok) {
      const message =
        typeof payload === "object" &&
        payload !== null &&
        "error" in payload &&
        typeof (payload as { error?: { message?: unknown } }).error?.message ===
          "string"
          ? String((payload as { error: { message: string } }).error.message)
          : text || `HTTP ${response.status}`;
      return { ok: false, status: response.status, message };
    }
    return { ok: true, data: payload as T };
  } catch (error) {
    return {
      ok: false,
      status: 0,
      message: error instanceof Error ? error.message : String(error),
    };
  }
}

const jsonHeaders = { "Content-Type": "application/json" };

export async function listMiniApps(
  query = "",
): Promise<APIResult<{ items: MiniAppCatalogEntry[] }>> {
  const suffix = query.trim() ? `?q=${encodeURIComponent(query.trim())}` : "";
  return request(`/foxxycode/miniapps${suffix}`);
}

export async function getMiniAppDraft(
  id: string,
): Promise<APIResult<MiniAppDocument>> {
  return request(`/foxxycode/miniapps/${encodeURIComponent(id)}/draft`);
}

export async function getMiniAppSource(
  id: string,
): Promise<APIResult<SourceEvidence>> {
  return request(
    `/foxxycode/miniapps/${encodeURIComponent(id)}/authoring/source`,
  );
}

export async function putMiniAppDraft(
  app: MiniAppDocument,
): Promise<APIResult<MiniAppDocument>> {
  return request(`/foxxycode/miniapps/${encodeURIComponent(app.id)}/draft`, {
    method: "PUT",
    headers: jsonHeaders,
    body: JSON.stringify(app),
  });
}

export async function createMiniApp(
  app: MiniAppDocument,
): Promise<APIResult<MiniAppDocument>> {
  return request("/foxxycode/miniapps", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(app),
  });
}

export async function distillSession(
  sessionId: string,
): Promise<APIResult<DistillationJob>> {
  return request(
    `/foxxycode/sessions/${encodeURIComponent(sessionId)}/miniapps/distill`,
    { method: "POST" },
  );
}

export async function getDistillation(
  id: string,
): Promise<APIResult<DistillationJob>> {
  return request(`/foxxycode/miniapp-distillations/${encodeURIComponent(id)}`);
}

export async function generateExpectedResult(
  app: MiniAppDocument,
  expectations: string,
): Promise<APIResult<ExpectedResultGeneration>> {
  return request(
    `/foxxycode/miniapps/${encodeURIComponent(app.id)}/expected-result`,
    {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ expectations, draft: app }),
    },
  );
}

export async function setMiniAppModelBinding(
  app: MiniAppDocument,
  modelRef: string,
): Promise<APIResult<MiniAppDocument>> {
  return request(
    `/foxxycode/miniapps/${encodeURIComponent(app.id)}/model-binding`,
    {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ model_ref: modelRef, draft: app }),
    },
  );
}

export async function editMiniAppWithAssistant(
  app: MiniAppDocument,
  message: string,
  history: MiniAppAuthoringTurn[],
): Promise<APIResult<MiniAppAuthoringResult>> {
  return request(
    `/foxxycode/miniapps/${encodeURIComponent(app.id)}/authoring/chat`,
    {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ message, history, draft: app }),
    },
  );
}

export async function testMiniApp(
  id: string,
  inputs: Record<string, unknown>,
  confirmations: Record<string, boolean> = {},
): Promise<APIResult<MiniAppRun>> {
  return request(`/foxxycode/miniapps/${encodeURIComponent(id)}/test-runs`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ inputs, confirmations }),
  });
}

export async function runMiniApp(
  id: string,
  version: string,
  inputs: Record<string, unknown>,
  confirmations: Record<string, boolean> = {},
): Promise<APIResult<MiniAppRun>> {
  return request(
    `/foxxycode/miniapps/${encodeURIComponent(id)}/versions/${encodeURIComponent(version)}/runs`,
    {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ inputs, confirmations }),
    },
  );
}

export async function releaseMiniApp(
  id: string,
  version: string,
): Promise<APIResult<MiniAppDocument>> {
  return request(`/foxxycode/miniapps/${encodeURIComponent(id)}/release`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ version }),
  });
}
