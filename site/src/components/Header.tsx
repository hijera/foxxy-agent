import { useState } from "react";
import markDark from "../../../docs/assets/foxxycode-logo-mark.svg";
import markLight from "../../../docs/assets/foxxycode-logo-mark-light.svg";
import { usePrefs, type Lang } from "../app/prefs";
import { format, links, pages, release } from "../app/data";
import { GitHubIcon, MenuIcon, MoonIcon, SunIcon } from "./Icons";

export type PageId = "landing" | "compare" | "changelog";

export function Logo() {
  const { m } = usePrefs();
  return (
    <a className="brand" href={pages.home} aria-label={m.nav.home}>
      <img className="brand-mark brand-mark--dark" src={markDark} alt="" width="30" height="30" />
      <img className="brand-mark brand-mark--light" src={markLight} alt="" width="30" height="30" />
      <span className="brand-name">FoxxyCode</span>
    </a>
  );
}

export function Header({ page }: { page: PageId }) {
  const { m, lang, setLang, theme, toggleTheme } = usePrefs();
  const [open, setOpen] = useState(false);
  const home = page === "landing" ? "" : pages.home;
  const navItems = [
    { href: `${home}#features`, label: m.nav.features },
    { href: `${home}#install`, label: m.nav.install },
    { href: `${home}#demos`, label: m.nav.demos },
    { href: pages.compare, label: m.nav.compare, current: page === "compare" },
    { href: pages.changelog, label: m.nav.changelog, current: page === "changelog" },
    { href: links.docs, label: m.nav.docs, external: true },
  ];

  return (
    <header className="site-header">
      <div className="container header-row">
        <Logo />
        <button
          type="button"
          className="icon-btn menu-btn"
          aria-label={m.nav.menu}
          aria-expanded={open}
          aria-controls="site-nav"
          onClick={() => setOpen((v) => !v)}
        >
          <MenuIcon />
        </button>
        <nav id="site-nav" className={`site-nav${open ? " is-open" : ""}`} aria-label={m.nav.menu}>
          <ul>
            {navItems.map((item) => (
              <li key={item.href}>
                <a
                  href={item.href}
                  aria-current={item.current ? "page" : undefined}
                  {...(item.external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
                  onClick={() => setOpen(false)}
                >
                  {item.label}
                </a>
              </li>
            ))}
          </ul>
          <div className="header-tools">
            <a
              className="version-chip"
              href={release.latest?.url ?? links.releases}
              target="_blank"
              rel="noopener noreferrer"
              title={release.latest ? format(m.nav.latestRelease, { version: release.latest.version }) : m.nav.github}
            >
              <GitHubIcon />
              <span>{release.latest ? `v${release.latest.version}` : m.nav.github}</span>
            </a>
            <div className="lang-switch" role="group" aria-label={m.nav.language}>
              {(["ru", "en"] as Lang[]).map((code) => (
                <button
                  key={code}
                  type="button"
                  lang={code}
                  aria-pressed={lang === code}
                  onClick={() => setLang(code)}
                >
                  {code.toUpperCase()}
                </button>
              ))}
            </div>
            <button
              type="button"
              className="icon-btn"
              onClick={toggleTheme}
              aria-label={theme === "dark" ? m.nav.toLight : m.nav.toDark}
              title={theme === "dark" ? m.nav.toLight : m.nav.toDark}
            >
              {theme === "dark" ? <SunIcon /> : <MoonIcon />}
            </button>
          </div>
        </nav>
      </div>
    </header>
  );
}

export function Footer() {
  const { m } = usePrefs();
  return (
    <footer className="site-footer">
      <div className="container footer-grid">
        <div className="footer-brand">
          <Logo />
          <p>{m.footer.tagline}</p>
        </div>
        <FooterColumn
          title={m.footer.project}
          items={[
            { href: links.repo, label: m.footer.source, external: true },
            { href: links.releases, label: m.footer.releases, external: true },
            { href: links.docs, label: m.footer.docs, external: true },
            { href: links.issues, label: m.footer.issues, external: true },
          ]}
        />
        <FooterColumn
          title={m.footer.pages}
          items={[
            { href: pages.home, label: m.footer.home },
            { href: pages.compare, label: m.footer.compare },
            { href: pages.changelog, label: m.footer.changelog },
          ]}
        />
        <FooterColumn
          title={m.footer.upstream}
          items={[
            { href: links.coddySite, label: m.footer.coddySite, external: true },
            { href: links.coddyRepo, label: m.footer.coddyRepo, external: true },
          ]}
        />
      </div>
    </footer>
  );
}

function FooterColumn({
  title,
  items,
}: {
  title: string;
  items: { href: string; label: string; external?: boolean }[];
}) {
  return (
    <div className="footer-col">
      <h2>{title}</h2>
      <ul>
        {items.map((item) => (
          <li key={item.href}>
            <a href={item.href} {...(item.external ? { target: "_blank", rel: "noopener noreferrer" } : {})}>
              {item.label}
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}
