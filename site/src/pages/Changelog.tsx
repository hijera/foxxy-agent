import { usePrefs } from "../app/prefs";
import { changelog, format, type MergedEntry } from "../app/data";
import { Blocks, InlineTokens } from "../components/Inline";
import { ExternalIcon } from "../components/Icons";

export function Changelog() {
  const { m, lang } = usePrefs();
  const versions = changelog.versions;
  return (
    <section className="section page-top">
      <div className="container narrow">
        <div className="section-head">
          <p className="eyebrow">{m.changelog.eyebrow}</p>
          <h1>{m.changelog.title}</h1>
          <p className="lead">{format(m.changelog.lead, { count: versions.length })}</p>
          <p>
            <a className="btn btn-ghost" href={changelog.releasesUrl} target="_blank" rel="noopener noreferrer">
              {m.changelog.releases}
              <ExternalIcon />
            </a>
          </p>
        </div>
        {versions.length === 0 ? <p className="empty">{m.changelog.empty}</p> : null}
        <ol className="release-list">
          {versions.map((version) => (
            <li key={version.version} className="release" id={`v${version.version}`}>
              <header className="release-head">
                <h2>
                  <a href={`#v${version.version}`}>{version.version}</a>
                </h2>
                <time dateTime={version.date}>
                  {new Date(`${version.date}T00:00:00Z`).toLocaleDateString(lang === "ru" ? "ru-RU" : "en-US", {
                    year: "numeric",
                    month: "long",
                    day: "numeric",
                    timeZone: "UTC",
                  })}
                </time>
                <a
                  className="release-link"
                  href={`${changelog.releasesUrl}/tag/${version.version}`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {format(m.changelog.releaseLink, { version: version.version })}
                  <ExternalIcon />
                </a>
              </header>
              {version.entries.map((entry, i) => (
                <ChangelogEntry key={i} entry={entry} />
              ))}
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}

function ChangelogEntry({ entry }: { entry: MergedEntry }) {
  const { m, lang } = usePrefs();
  const showEnglish = lang === "en" && entry.blocks.en !== null;
  const title = showEnglish ? entry.title.en : entry.title.ru;
  const blocks = showEnglish && entry.blocks.en ? entry.blocks.en : entry.blocks.ru;
  const surfaces =
    entry.surfaces.length === 2 ? [m.changelog.both] : entry.surfaces.map((surface) => m.changelog[surface]);
  return (
    <article className="entry" lang={showEnglish || lang === "ru" ? lang : "ru"}>
      <div className="entry-badges">
        {surfaces.map((label) => (
          <span key={label} className="tag">
            {label}
          </span>
        ))}
      </div>
      {title ? (
        <h3>
          <InlineTokens tokens={title} />
        </h3>
      ) : null}
      <div className="prose">
        <Blocks blocks={blocks} />
      </div>
      {lang === "en" && !showEnglish ? <p className="entry-note">{m.changelog.untranslated}</p> : null}
    </article>
  );
}
