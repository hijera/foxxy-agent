export type ProjectInfo = {
  path: string;
  source: "project" | "default";
  native_picker: boolean;
};

export type RecentProject = {
  path: string;
  name: string;
  last_opened_at?: string;
  exists: boolean;
};

export type PickFolderResult =
  | { path: string; cancelled: boolean }
  | { unavailable: true }
  | null;

export async function fetchProject(): Promise<ProjectInfo | null> {
  try {
    const res = await fetch("/foxxycode/project");
    if (!res.ok) return null;
    return (await res.json()) as ProjectInfo;
  } catch {
    return null;
  }
}

export async function putProject(
  path: string,
): Promise<{ ok: true; info: ProjectInfo } | { ok: false; error: string }> {
  try {
    const res = await fetch("/foxxycode/project", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    });
    if (!res.ok) {
      let msg = `HTTP ${res.status}`;
      try {
        const j = (await res.json()) as { error?: { message?: string } };
        if (j.error?.message) msg = j.error.message;
      } catch {
        // keep status message
      }
      return { ok: false, error: msg };
    }
    return { ok: true, info: (await res.json()) as ProjectInfo };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "network error" };
  }
}

export async function fetchRecentProjects(): Promise<RecentProject[]> {
  try {
    const res = await fetch("/foxxycode/projects/recent");
    if (!res.ok) return [];
    const j = (await res.json()) as { data?: RecentProject[] };
    return j.data || [];
  } catch {
    return [];
  }
}

export async function pickFolder(): Promise<PickFolderResult> {
  try {
    const res = await fetch("/foxxycode/project/pick-folder", {
      method: "POST",
    });
    if (res.status === 501) return { unavailable: true };
    if (!res.ok) return null;
    const j = (await res.json()) as { path?: string; cancelled?: boolean };
    return { path: j.path || "", cancelled: !!j.cancelled };
  } catch {
    return null;
  }
}

/** Last path segment of a project path for compact display. */
export function projectBasename(path: string): string {
  const trimmed = (path || "").replace(/[\\/]+$/, "");
  const i = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  return i >= 0 ? trimmed.slice(i + 1) : trimmed;
}
