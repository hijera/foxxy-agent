import { usePrefs, type Messages } from "../app/prefs";
import { cellText, compare, type Cell, type CompareTable, type Harness } from "../app/compare";
import { links, pages } from "../app/data";
import { Text } from "../components/Inline";
import { ArrowIcon, ExternalIcon } from "../components/Icons";
import "../styles/compare.css";

export function Compare() {
  const { m } = usePrefs();
  const c = m.compare;
  return (
    <>
      <section className="section page-top">
        <div className="container">
          <div className="section-head compare-head">
            <p className="eyebrow">{c.eyebrow}</p>
            <h1>{c.title}</h1>
            <p className="lead">{c.lead}</p>
            <p className="compiled">
              <Text>{c.compiled}</Text>
            </p>
          </div>

          <h2 className="subhead">{c.glanceTitle}</h2>
          <ul className="glance">
            {c.glance.map((g) => (
              <li key={g.value} className="card">
                <strong>{g.value}</strong>
                <span>{g.label}</span>
              </li>
            ))}
          </ul>

          <div className="compare-prose prose">
            <h2 className="subhead">{c.camps.title}</h2>
            {c.camps.paragraphs.map((p) => (
              <p key={p}>
                <Text>{p}</Text>
              </p>
            ))}
          </div>

          <Legend m={m} />
        </div>
      </section>

      {compare.tables.map((table) => (
        <TableSection key={table.id} table={table} />
      ))}

      <section className="section">
        <div className="container">
          <h2 className="subhead">{c.verdict.title}</h2>
          <div className="verdict">
            <VerdictColumn title={c.verdict.strengths} items={c.verdict.strengthItems} tone="yes" />
            <VerdictColumn title={c.verdict.gaps} items={c.verdict.gapItems} tone="no" />
          </div>

          <div className="compare-prose prose">
            <h2 className="subhead">{c.pick.title}</h2>
            {c.pick.paragraphs.map((p) => (
              <p key={p}>
                <Text>{p}</Text>
              </p>
            ))}
          </div>

          <div className="card compare-cta">
            <div>
              <h2>{c.cta.title}</h2>
              <p>{c.cta.text}</p>
            </div>
            <div className="compare-cta-actions">
              <a className="btn btn-primary" href={`${pages.home}#install`}>
                {c.cta.install}
                <ArrowIcon />
              </a>
              <a className="btn btn-ghost" href={links.repo} target="_blank" rel="noopener noreferrer">
                {c.cta.source}
                <ExternalIcon />
              </a>
            </div>
          </div>

          <div className="compare-prose prose sources" id="sources">
            <h2 className="subhead">{c.sources.title}</h2>
            <p>
              <Text>{c.sources.text}</Text>
            </p>
            <ul>
              {compare.sources.map((s) => (
                <li key={s.url}>
                  <a href={s.url} target="_blank" rel="noopener noreferrer">
                    {s.label}
                  </a>
                </li>
              ))}
            </ul>
            <p>
              <Text>{c.sources.fix}</Text>
            </p>
          </div>
        </div>
      </section>
    </>
  );
}

function Legend({ m }: { m: Messages }) {
  const l = m.compare.legend;
  return (
    <div className="legend">
      <h2 className="subhead">{l.title}</h2>
      <ul>
        {(["yes", "partial", "no", "unknown"] as const).map((s) => (
          <li key={s}>
            <span className={`dot dot-${s}`} aria-hidden="true" />
            {l[s]}
          </li>
        ))}
      </ul>
      <p>{l.note}</p>
    </div>
  );
}

function TableSection({ table }: { table: CompareTable }) {
  const { m, lang } = usePrefs();
  return (
    <section className="section compare-table-section" id={table.id}>
      <div className="container">
        <h2 className="table-title">{table.title[lang]}</h2>
        <p className="table-lead">{table.lead[lang]}</p>
        <div className="table-scroll" tabIndex={0} role="region" aria-label={table.title[lang]}>
          <table className="compare-table">
            <thead>
              <tr>
                <th scope="col">{m.compare.harness}</th>
                {table.columns.map((col) => (
                  <th key={col.id} scope="col">
                    {col.label[lang]}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {compare.harnesses.map((h) => (
                <tr key={h.id} className={`row-${h.role}`}>
                  <HarnessCell harness={h} />
                  {table.columns.map((col) => {
                    const cell = table.rows[h.id]?.[col.id];
                    return cell ? <ValueCell key={col.id} cell={cell} /> : <td key={col.id} />;
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

function HarnessCell({ harness }: { harness: Harness }) {
  const { m, lang } = usePrefs();
  return (
    <th scope="row">
      <a href={harness.url} target="_blank" rel="noopener noreferrer">
        {harness.name}
      </a>
      {harness.role === "self" ? <span className="tag">{m.compare.youAreHere}</span> : null}
      {harness.role === "upstream" ? <span className="tag tag-muted">{m.compare.upstreamBadge}</span> : null}
      {harness.note ? <span className="harness-note">{harness.note[lang]}</span> : null}
    </th>
  );
}

function ValueCell({ cell }: { cell: Cell }) {
  const { m, lang } = usePrefs();
  const text = cellText(cell, lang);
  const legend = m.compare.legend;
  const label = cell.s === "info" ? null : legend[cell.s];
  return (
    <td className={`cell cell-${cell.s}`}>
      <span className="cell-inner">
        {cell.s !== "info" ? <span className={`dot dot-${cell.s}`} aria-hidden="true" /> : null}
        <span className="cell-text">{text ?? label}</span>
        {text && label ? <span className="visually-hidden"> ({label})</span> : null}
      </span>
    </td>
  );
}

function VerdictColumn({
  title,
  items,
  tone,
}: {
  title: string;
  items: { title: string; text: string }[];
  tone: "yes" | "no";
}) {
  return (
    <div className={`card verdict-col verdict-${tone}`}>
      <h3>{title}</h3>
      <dl>
        {items.map((item) => (
          <div key={item.title}>
            <dt>{item.title}</dt>
            <dd>
              <Text>{item.text}</Text>
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
