import { useState } from 'react';

export function Composer(props: {
  value: string;
  isEmpty: boolean;
  mode: string;
  modes: string[];
  onModeChange: (mode: string) => void;
  onChange: (v: string) => void;
  onSend: (text: string) => void;
}) {
  const sendDisabled = props.value.trim() === '';
  const [modeOpen, setModeOpen] = useState(false);
  const modeLabel = (props.mode || 'agent').slice(0, 1).toUpperCase() + (props.mode || 'agent').slice(1);

  return (
    <footer className="composer-wrap">
      <label className="sr-only" htmlFor="composer">
        Message
      </label>
      <div className="composer-card">
        <textarea
          id="composer"
          rows={props.isEmpty ? 5 : 2}
          placeholder={props.isEmpty ? 'Ask anything...' : 'Message Coddy'}
          autoComplete="off"
          value={props.value}
          onChange={(ev) => props.onChange(ev.target.value)}
          onKeyDown={(ev) => {
            if (ev.key === 'Enter' && !ev.shiftKey) {
              ev.preventDefault();
              const txt = props.value.trim();
              if (!txt) {
                return;
              }
              props.onSend(txt);
            }
          }}
        />

        <div className="composer-bar">
          <div className="composer-tabs" aria-label="Composer options">
            <div className="mode">
              <button
                type="button"
                className={`composer-tab mode-btn ${props.mode === 'plan' ? 'mode-plan' : 'mode-agent'}`}
                aria-label="Mode"
                title="Mode"
                onClick={() => setModeOpen((v) => !v)}
              >
                {modeLabel}
              </button>
              {modeOpen ? (
                <div className="mode-menu" role="menu">
                  {props.modes.map((m) => {
                    const label = m.slice(0, 1).toUpperCase() + m.slice(1);
                    return (
                    <button
                      key={m}
                      type="button"
                      role="menuitem"
                      className={`mode-item ${m === props.mode ? 'is-selected' : ''}`}
                      onClick={() => {
                        props.onModeChange(m);
                        setModeOpen(false);
                      }}
                    >
                      {label}
                    </button>
                    );
                  })}
                </div>
              ) : null}
            </div>
          </div>

          <div className="composer-bar-actions">
            <button
              type="button"
              className="composer-icon composer-send"
              id="btn-send"
              aria-label="Send"
              disabled={sendDisabled}
              onClick={() => {
                const txt = props.value.trim();
                if (!txt) {
                  return;
                }
                props.onSend(txt);
              }}
            >
              <span aria-hidden="true">➤</span>
            </button>
          </div>
        </div>
      </div>
    </footer>
  );
}
