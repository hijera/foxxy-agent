import { useCallback, useEffect, useState } from "react";

type InstalledSkill = {
  name: string;
  description: string;
  file_path: string;
  always_apply: boolean;
  globs?: string[];
  disabled: boolean;
};

async function fetchInstalled(): Promise<InstalledSkill[]> {
  const res = await fetch("/coddy/skills");
  if (!res.ok) return [];
  const data = (await res.json()) as { items?: InstalledSkill[] };
  return data.items ?? [];
}

async function apiPost(path: string): Promise<{ ok: boolean; error?: string }> {
  const res = await fetch(path, { method: "POST" });
  if (!res.ok) {
    try {
      const j = (await res.json()) as { error?: { message?: string } };
      return { ok: false, error: j.error?.message || `HTTP ${res.status}` };
    } catch {
      return { ok: false, error: `HTTP ${res.status}` };
    }
  }
  return { ok: true };
}

function IconPlug() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none"
      stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M7 22H4a2 2 0 0 1-2-2v-3a2 2 0 0 0-2 0V7a2 2 0 0 0 2 0H7" />
      <path d="M15 7h4a2 2 0 0 1 2 2v4a2 2 0 0 0 0 2v3a2 2 0 0 1-2 2h-3" />
      <line x1="12" y1="2" x2="12" y2="22" />
    </svg>
  );
}

export function SkillsPanel(props: { onClose: () => void }) {
  const [installed, setInstalled] = useState<InstalledSkill[]>([]);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const loadInstalled = useCallback(async () => {
    setLoading(true);
    const data = await fetchInstalled();
    setInstalled(data);
    setLoading(false);
  }, []);

  useEffect(() => {
    void loadInstalled();
  }, [loadInstalled]);

  const withBusy = async (key: string, fn: () => Promise<void>) => {
    setBusy((p) => ({ ...p, [key]: true }));
    setError(null);
    try {
      await fn();
    } finally {
      setBusy((p) => ({ ...p, [key]: false }));
    }
  };

  const onToggle = (skill: InstalledSkill) => {
    void withBusy(skill.name, async () => {
      const action = skill.disabled ? "enable" : "disable";
      const res = await apiPost(`/coddy/skills/${encodeURIComponent(skill.name)}/${action}`);
      if (!res.ok) {
        setError(res.error || `Failed to ${action}`);
        return;
      }
      await loadInstalled();
    });
  };

  return (
    <aside className="sessions settings drawer" aria-label="Skills" data-testid="skills-panel" data-variant="drawer">
      <div className="sessions-head">
        <span>Skills</span>
        <button
          type="button"
          className="sessions-close"
          aria-label="Close skills panel"
          data-testid="skills-panel-close"
          onClick={props.onClose}
        >
          ×
        </button>
      </div>

      <div className="settings-lead-pane">
        <p className="settings-lead">
          Install skills via <code>npx skills</code> or <code>npx skillsbd</code> — they land in{" "}
          <code>~/.agents/skills/</code> and are picked up automatically.
        </p>
        {error ? <p className="settings-error">{error}</p> : null}
      </div>

      <div className="settings-stack">
        <div className="settings-scroll">
          <div className="settings-body">
            {loading ? (
              <p className="settings-muted">Loading…</p>
            ) : installed.length === 0 ? (
              <p className="settings-muted">No skills found. Use <code>npx skills</code> or <code>npx skillsbd</code> to install.</p>
            ) : (
              <ul className="skills-list" style={{ listStyle: "none", padding: 0, margin: 0 }}>
                {installed.map((sk) => (
                  <li
                    key={sk.name}
                    className="skills-list-item"
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: "8px",
                      padding: "8px 0",
                      borderBottom: "1px solid var(--surface2, #333)",
                      opacity: sk.disabled ? 0.5 : 1,
                    }}
                  >
                    <IconPlug />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 600, fontSize: "0.85rem" }}>{sk.name}</div>
                      {sk.description ? (
                        <div style={{ fontSize: "0.78rem", color: "var(--text2, #888)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                          {sk.description}
                        </div>
                      ) : null}
                    </div>
                    <button
                      type="button"
                      className="settings-btn"
                      style={{ fontSize: "0.75rem", padding: "2px 8px" }}
                      disabled={!!busy[sk.name]}
                      onClick={() => onToggle(sk)}
                      title={sk.disabled ? "Enable" : "Disable"}
                    >
                      {sk.disabled ? "Enable" : "Disable"}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </div>
    </aside>
  );
}
