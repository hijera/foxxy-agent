import type {
  JobsListResponse,
  SchedulerJob,
  SchedulerJobCreate,
  SchedulerJobPatch,
} from "./types";
import { t } from "../i18n/i18n";

async function readErrorMessage(res: Response): Promise<string> {
  try {
    const j = (await res.json()) as {
      error?: { message?: string };
    };
    const m = j?.error?.message;
    if (typeof m === "string" && m.trim()) {
      return m.trim();
    }
  } catch {
    /* ignore */
  }
  return `HTTP ${res.status}`;
}

export type ApiResult<T> =
  | { ok: true; data: T }
  | { ok: false; status: number; message: string };

/**
 * The jobs list is polled on a timer while the scheduler drawer is open, so a server
 * that is down or restarting must degrade into a normal error result rather than an
 * unhandled rejection once per tick — the IDE panels paint those as a red overlay.
 */
async function request(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response | null> {
  try {
    return await fetch(input, init);
  } catch {
    return null;
  }
}

// Built per call rather than once at module load, so a locale switch is picked up
// without a reload. status 0 means the server gave us no answer at all.
function offline<T>(): ApiResult<T> {
  return { ok: false, status: 0, message: t("scheduler.offline") };
}

async function parseJson<T>(res: Response | null): Promise<ApiResult<T>> {
  if (!res) {
    return offline();
  }
  if (!res.ok) {
    return {
      ok: false,
      status: res.status,
      message: await readErrorMessage(res),
    };
  }
  const data = (await res.json()) as T;
  return { ok: true, data };
}

export async function schedulerListJobs(
  includeBody?: boolean,
): Promise<ApiResult<JobsListResponse>> {
  const sp = new URLSearchParams();
  if (includeBody) {
    sp.set("include_body", "true");
  }
  const q = sp.toString();
  const path = q ? `/foxxycode/scheduler/jobs?${q}` : "/foxxycode/scheduler/jobs";
  const res = await request(path);
  return parseJson<JobsListResponse>(res);
}

export async function schedulerGetJob(
  jobId: string,
): Promise<ApiResult<SchedulerJob>> {
  const res = await request(
    `/foxxycode/scheduler/jobs/${encodeURIComponent(jobId)}`,
  );
  return parseJson<SchedulerJob>(res);
}

export async function schedulerCreateJob(
  body: SchedulerJobCreate,
): Promise<ApiResult<{ object?: string; job_id?: string }>> {
  const res = await request("/foxxycode/scheduler/jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return parseJson(res);
}

export async function schedulerPatchJob(
  jobId: string,
  patch: SchedulerJobPatch,
): Promise<
  ApiResult<{ object?: string; job_id?: string }>
> {
  const res = await request(
    `/foxxycode/scheduler/jobs/${encodeURIComponent(jobId)}`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    },
  );
  return parseJson(res);
}

export async function schedulerDeleteJob(
  jobId: string,
): Promise<ApiResult<null>> {
  const res = await request(
    `/foxxycode/scheduler/jobs/${encodeURIComponent(jobId)}`,
    { method: "DELETE" },
  );
  if (!res) {
    return offline();
  }
  if (res.ok && (res.status === 204 || res.status === 200)) {
    return { ok: true, data: null };
  }
  return {
    ok: false,
    status: res.status,
    message: await readErrorMessage(res),
  };
}

export async function schedulerPauseJob(
  jobId: string,
): Promise<ApiResult<{ object?: string; job_id?: string }>> {
  const res = await request(
    `/foxxycode/scheduler/jobs/${encodeURIComponent(jobId)}/pause`,
    { method: "POST" },
  );
  return parseJson(res);
}

export async function schedulerResumeJob(
  jobId: string,
): Promise<ApiResult<{ object?: string; job_id?: string }>> {
  const res = await request(
    `/foxxycode/scheduler/jobs/${encodeURIComponent(jobId)}/resume`,
    { method: "POST" },
  );
  return parseJson(res);
}

export async function schedulerRunJob(
  jobId: string,
): Promise<
  ApiResult<{ object?: string; job_id?: string; status?: string }>
> {
  const res = await request(
    `/foxxycode/scheduler/jobs/${encodeURIComponent(jobId)}/run`,
    { method: "POST" },
  );
  return parseJson(res);
}

export async function schedulerCancelJob(
  jobId: string,
): Promise<
  ApiResult<{ object?: string; job_id?: string; cancelled?: boolean }>
> {
  const res = await request(
    `/foxxycode/scheduler/jobs/${encodeURIComponent(jobId)}/cancel`,
    { method: "POST" },
  );
  return parseJson(res);
}
