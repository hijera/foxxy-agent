export function NavRail(props: {
  onNewChat: () => void;
  onToggleMenu: () => void;
  menuOpen: boolean;
  navWide: boolean;
  onToggleNavWide: () => void;
}) {
  return (
    <aside className="rail" aria-label="Nav">
      <div className="rail-pill">
        <button type="button" className="rail-icon" title="Menu" aria-label="Menu" onClick={props.onToggleMenu}>
          <span aria-hidden="true">≡</span>
        </button>

        <button type="button" className="rail-brand" aria-label="Coddy chat" onClick={props.onNewChat}>
          <div className="rail-brand-text">
            <div className="rail-brand-title">Coddy</div>
            <div className="rail-brand-sub">chat</div>
          </div>
        </button>

        <div className="rail-spacer" />

        <button type="button" className="rail-icon" aria-label="Toggle nav width" title="Toggle nav" onClick={props.onToggleNavWide}>
          <span aria-hidden="true">{props.navWide ? '⟷' : '↔'}</span>
        </button>

        <a className="rail-icon rail-link" href="https://github.com/coddy-project/coddy-agent" target="_blank" rel="noopener" aria-label="GitHub">
          <span aria-hidden="true">GH</span>
        </a>
        <a className="rail-icon rail-link" href="/docs/" aria-label="API docs">
          <span aria-hidden="true">API</span>
        </a>
      </div>
    </aside>
  );
}

