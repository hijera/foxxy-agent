import { useEffect, useRef, useState } from 'react';
import type { SessionRow } from './types';

export function SessionsSidebar(props: {
  sessionId: string;
  sessions: SessionRow[];
  variant?: 'dock' | 'drawer';
  open?: boolean;
  onClose?: () => void;
  onPick: (id: string) => void;
  onRename: (id: string) => void;
  onTitleSave?: (id: string, title: string) => void;
  onDelete: (id: string) => void;
  onLoadMore: () => void;
}) {
  const variant = props.variant || 'dock';
  const isOpen = variant === 'dock' ? true : !!props.open;
  const [editingId, setEditingId] = useState<string | null>(null);
  const [titleDraft, setTitleDraft] = useState('');
  const titleRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (editingId) {
      titleRef.current?.focus();
      titleRef.current?.select();
    }
  }, [editingId]);

  if (!isOpen) {
    return null;
  }

  return (
    <aside className={`sessions ${variant === 'drawer' ? 'drawer' : 'dock'}`} aria-label="Sessions">
      <div className="sessions-head">
        <span>Chats</span>
        {variant === 'drawer' ? (
          <button type="button" className="sessions-close" aria-label="Close" onClick={props.onClose}>
            ×
          </button>
        ) : null}
      </div>
      <div className="session-list" id="session-list">
        {props.sessions.map((s) => (
          <div
            key={s.id}
            className={`session-item ${s.id === props.sessionId ? 'active' : ''}`}
            onClick={() => {
              if (editingId === s.id) {
                return;
              }
              props.onPick(s.id);
              props.onClose?.();
            }}
          >
            <div className="session-row">
              {editingId === s.id ? (
                <input
                  ref={titleRef}
                  className="session-title-input"
                  value={titleDraft}
                  onMouseDown={(ev) => ev.stopPropagation()}
                  onChange={(e) => setTitleDraft(e.target.value)}
                  onBlur={() => {
                    const t = titleDraft.trim();
                    setEditingId(null);
                    if (t && props.onTitleSave) {
                      props.onTitleSave(s.id, t);
                    }
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      (e.target as HTMLInputElement).blur();
                    }
                    if (e.key === 'Escape') {
                      setEditingId(null);
                      setTitleDraft(s.title || '');
                    }
                  }}
                />
              ) : (
                <button
                  type="button"
                  className="session-title-btn"
                  onMouseDown={(ev) => ev.stopPropagation()}
                  onClick={() => {
                    setEditingId(s.id);
                    setTitleDraft(s.title || '');
                  }}
                  aria-label="Rename chat"
                  title={s.title || 'New chat'}
                >
                  {s.title || 'New chat'}
                </button>
              )}
              <button
                className="session-trash"
                type="button"
                aria-label="Delete chat"
                title="Delete"
                onMouseDown={(ev) => ev.stopPropagation()}
                onClick={() => {
                  const ok = window.confirm('Delete chat');
                  if (ok) props.onDelete(s.id);
                }}
              >
                🗑
              </button>
            </div>
          </div>
        ))}
      </div>
      <div className="sessions-foot">
        <button type="button" className="link" id="btn-load-more" onClick={props.onLoadMore}>
          Load more
        </button>
      </div>
    </aside>
  );
}
