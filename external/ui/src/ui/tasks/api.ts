import { t } from "../i18n/i18n";
import type {
  BackgroundTaskListResponse,
  BackgroundTaskResponse,
} from "./types";

export type TasksApiResult<T> =
  | { ok: true; data: T }
  | { ok: false; status: number; message: string };

async function readErrorMessage(res: Response): Promise<string> {
  try {
    const j = (await res.json()) as { error?: { message?: string } };
    const m = j?.error?.message;
    if (typeof m === "string" && m.trim()) {
      return m.trim();
    }
  } catch {
    /* ignore */
  }
  return `HTTP ${res.status}`;
}

async function parseJson<T>(res: Response): Promise<TasksApiResult<T>> {
  if (!res.ok) {
    return {
      ok: false,
      status: res.status,
      message: await readErrorMessage(res),
    };
  }
  return { ok: true, data: (await res.json()) as T };
}

/**
 * The tasks drawer polls on a timer, so a server that is down or restarting must
 * degrade into a normal error result rather than an unhandled rejection once per
 * tick.
 */
async function request(
  path: string,
  init: RequestInit,
): Promise<Response | null> {
  try {
    return await fetch(path, init);
  } catch {
    return null;
  }
}

// Built per call rather than once at module load, so a locale switch is picked
// up without a reload.
function offline<T>(): TasksApiResult<T> {
  return { ok: false, status: 0, message: t("tasks.offline") };
}

function basePath(sessionId: string): string {
  return `/foxxycode/sessions/${encodeURIComponent(sessionId)}/background-tasks`;
}

export async function listBackgroundTasks(
  sessionId: string,
): Promise<TasksApiResult<BackgroundTaskListResponse>> {
  const res = await request(basePath(sessionId), {
    headers: { "X-FoxxyCode-Session-ID": sessionId },
  });
  return res ? parseJson<BackgroundTaskListResponse>(res) : offline();
}

export async function getBackgroundTask(
  sessionId: string,
  taskId: string,
): Promise<TasksApiResult<BackgroundTaskResponse>> {
  const res = await request(`${basePath(sessionId)}/${encodeURIComponent(taskId)}`, {
    headers: { "X-FoxxyCode-Session-ID": sessionId },
  });
  return res ? parseJson<BackgroundTaskResponse>(res) : offline();
}

export async function clearFinishedBackgroundTasks(
  sessionId: string,
): Promise<TasksApiResult<{ cleared: number }>> {
  const res = await request(basePath(sessionId), {
    method: "DELETE",
    headers: { "X-FoxxyCode-Session-ID": sessionId },
  });
  return res ? parseJson<{ cleared: number }>(res) : offline();
}

export async function stopBackgroundTask(
  sessionId: string,
  taskId: string,
): Promise<TasksApiResult<BackgroundTaskResponse>> {
  const res = await request(
    `${basePath(sessionId)}/${encodeURIComponent(taskId)}/stop`,
    { method: "POST", headers: { "X-FoxxyCode-Session-ID": sessionId } },
  );
  return res ? parseJson<BackgroundTaskResponse>(res) : offline();
}
