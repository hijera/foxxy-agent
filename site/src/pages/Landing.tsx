import { useState, type ReactNode } from "react";
import { usePrefs } from "../app/prefs";
import { format, formatSize, links, pages, release, repoFile, type DownloadKind } from "../app/data";
import { DEMO_SURFACES, pickShot, shots, shotUrl, type DemoSurface } from "../app/demos";
import { Text } from "../components/Inline";
import { CopyField } from "../components/CopyField";
import {
  ArrowIcon,
  DownloadIcon,
  ExternalIcon,
  PlugIcon,
  PuzzleIcon,
  TerminalIcon,
  WindowIcon,
} from "../components/Icons";

export function Landing() {
  return (
    <>
      <Hero />
      <Install />
      <Demos />
      <About />
      <Features />
      <CompareTeaser />
      <Faq />
    </>
  );
}

function Section({ id, className, children }: { id: string; className?: string; children: ReactNode }) {
  return (
    <section id={id} className={`section${className ? ` ${className}` : ""}`}>
      <div className="container">{children}</div>
    </section>
  );
}

function SectionHead({ eyebrow, title, lead }: { eyebrow?: string; title: string; lead?: string }) {
  return (
    <div className="section-head">
      {eyebrow ? <p className="eyebrow">{eyebrow}</p> : null}
      <h2>{title}</h2>
      {lead ? (
        <p className="lead">
          <Text>{lead}</Text>
        </p>
      ) : null}
    </div>
  );
}

function Hero() {
  const { m } = usePrefs();
  return (
    <section className="hero" id="top">
      <div className="container hero-inner">
        <h1>
          {m.hero.title}
          <span className="hero-accent">{m.hero.titleAccent}</span>
        </h1>
        <p className="hero-lead">{m.hero.lead}</p>
        <ul className="chips">
          {m.hero.chips.map((chip) => (
            <li key={chip} className="chip">
              {chip}
            </li>
          ))}
        </ul>
        <div className="hero-actions">
          <a className="btn btn-primary" href="#install">
            <DownloadIcon />
            {m.hero.primary}
          </a>
          <a className="btn btn-ghost" href="#features">
            {m.hero.secondary}
            <ArrowIcon />
          </a>
        </div>
        <p className="hero-based">
          <Text>{m.hero.basedOn}</Text>
        </p>
      </div>
    </section>
  );
}

const CARD_ICONS: Record<DownloadKind, ReactNode> = {
  vscode: <PuzzleIcon />,
  intellij: <PlugIcon />,
  desktop: <WindowIcon />,
  cliWindows: <TerminalIcon />,
};

const CARD_ORDER: DownloadKind[] = ["vscode", "intellij", "desktop", "cliWindows"];

function Install() {
  const { m, lang } = usePrefs();
  return (
    <Section id="install" className="section-install">
      <SectionHead eyebrow={m.install.eyebrow} title={m.install.title} lead={m.install.lead} />
      <div className="install-grid">
        {CARD_ORDER.map((kind) => {
          const card = m.install.cards[kind];
          const download = release.downloads[kind];
          return (
            <article key={kind} className="card install-card" data-kind={kind}>
              <header className="install-card-head">
                <span className="install-icon">{CARD_ICONS[kind]}</span>
                <div>
                  <h3>{card.title}</h3>
                  <span className="tag">{card.tag}</span>
                </div>
              </header>
              <p className="install-text">
                <Text>{card.text}</Text>
              </p>
              <ol className="steps">
                {card.steps.map((step) => (
                  <li key={step}>
                    <Text>{step}</Text>
                  </li>
                ))}
              </ol>
              {kind === "intellij" ? (
                <CopyField value={release.intellijRepositoryUrl} label={m.install.cards.intellij.repository} />
              ) : null}
              <div className="install-foot">
                {download ? (
                  <>
                    <a className="btn btn-primary btn-block" href={download.url}>
                      <DownloadIcon />
                      {m.install.download}
                    </a>
                    <span className="install-meta">
                      {format(m.install.version, { version: download.version })} · {formatSize(download.size, lang)}
                    </span>
                  </>
                ) : (
                  <a
                    className="btn btn-ghost btn-block"
                    href={links.releases}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {m.install.unavailable}
                  </a>
                )}
              </div>
            </article>
          );
        })}
      </div>
      <p className="install-other">
        <span>{m.install.other}</span>
        <a href={links.releases} target="_blank" rel="noopener noreferrer">
          {m.install.otherCta}
          <ExternalIcon />
        </a>
      </p>
    </Section>
  );
}

function Demos() {
  const { m, theme, lang } = usePrefs();
  const available = DEMO_SURFACES.filter((s) => shots[s].length > 0);
  const [active, setActive] = useState<DemoSurface>(available[0] ?? "intellij");
  if (available.length === 0) return null;
  const shot = pickShot(shots[active], theme, lang);

  return (
    <Section id="demos" className="section-demos">
      <SectionHead title={m.demos.title} lead={m.demos.lead} />
      <div className="tabs" role="tablist" aria-label={m.demos.title}>
        {available.map((surface) => (
          <button
            key={surface}
            type="button"
            role="tab"
            id={`demo-tab-${surface}`}
            aria-selected={surface === active}
            aria-controls="demo-panel"
            className="tab"
            onClick={() => setActive(surface)}
          >
            {m.demos.tabs[surface]}
          </button>
        ))}
      </div>
      <figure className="demo-frame" id="demo-panel" role="tabpanel" aria-labelledby={`demo-tab-${active}`}>
        {shot ? (
          <img
            src={shotUrl(shot)}
            width={shot.width}
            height={shot.height}
            alt={m.demos.captions[active]}
            loading="lazy"
            decoding="async"
          />
        ) : (
          <p className="demo-missing">{m.demos.missing}</p>
        )}
        <figcaption>
          <Text>{m.demos.captions[active]}</Text>
        </figcaption>
      </figure>
    </Section>
  );
}

function About() {
  const { m } = usePrefs();
  return (
    <Section id="about" className="section-about">
      <SectionHead title={m.about.title} />
      <div className="prose about-text">
        {m.about.paragraphs.map((p) => (
          <p key={p}>
            <Text>{p}</Text>
          </p>
        ))}
      </div>
    </Section>
  );
}

function Features() {
  const { m } = usePrefs();
  return (
    <Section id="features" className="section-features">
      <SectionHead title={m.features.title} lead={m.features.lead} />
      {m.features.groups.map((group) => (
        <div key={group.id} className="feature-group">
          <h3 className="group-title">{group.title}</h3>
          <div className="feature-grid">
            {group.items.map((item) => (
              <a
                key={item.title}
                className="card feature-card"
                href={repoFile(item.doc)}
                target="_blank"
                rel="noopener noreferrer"
              >
                <h4>{item.title}</h4>
                <p>
                  <Text>{item.text}</Text>
                </p>
              </a>
            ))}
          </div>
        </div>
      ))}
      <div className="card inherited-card">
        <div>
          <h3>{m.features.inherited.title}</h3>
          <p>
            <Text>{m.features.inherited.text}</Text>
          </p>
        </div>
        <div className="inherited-links">
          <a className="btn btn-ghost" href={links.coddySite} target="_blank" rel="noopener noreferrer">
            {m.features.inherited.site}
            <ExternalIcon />
          </a>
          <a className="btn btn-ghost" href={links.coddyRepo} target="_blank" rel="noopener noreferrer">
            {m.features.inherited.repo}
            <ExternalIcon />
          </a>
        </div>
      </div>
      <p className="more-link">
        <a href={links.forkDoc} target="_blank" rel="noopener noreferrer">
          {m.features.more}
          <ArrowIcon />
        </a>
      </p>
    </Section>
  );
}

function CompareTeaser() {
  const { m } = usePrefs();
  return (
    <Section id="compare" className="section-compare">
      <div className="card compare-teaser">
        <SectionHead title={m.compareTeaser.title} lead={m.compareTeaser.lead} />
        <ul className="teaser-points">
          {m.compareTeaser.points.map((p) => (
            <li key={p}>
              <Text>{p}</Text>
            </li>
          ))}
        </ul>
        <a className="btn btn-primary" href={pages.compare}>
          {m.compareTeaser.cta}
          <ArrowIcon />
        </a>
      </div>
    </Section>
  );
}

function Faq() {
  const { m } = usePrefs();
  return (
    <Section id="faq" className="section-faq">
      <SectionHead title={m.faq.title} />
      <div className="faq-list">
        {m.faq.items.map((item) => (
          <details key={item.q} className="faq-item">
            <summary>{item.q}</summary>
            <p>
              <Text>{item.a}</Text>
            </p>
          </details>
        ))}
      </div>
    </Section>
  );
}
