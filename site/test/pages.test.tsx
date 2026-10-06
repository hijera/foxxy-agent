// Smoke renders of the three pages in both languages, against the data build-data.mjs baked
// (`npm run data:offline` in CI).
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Page } from "../src/app/Shell";
import { Landing } from "../src/pages/Landing";
import { Changelog } from "../src/pages/Changelog";
import { Compare } from "../src/pages/Compare";
import { changelog, release } from "../src/app/data";
import { compare } from "../src/app/compare";
import en from "../src/i18n/en.json";
import ru from "../src/i18n/ru.json";

afterEach(() => {
  cleanup();
  localStorage.clear();
  document.documentElement.lang = "en";
  delete document.documentElement.dataset.theme;
});

function renderPage(lang: "en" | "ru", page: "landing" | "compare" | "changelog") {
  document.documentElement.lang = lang;
  document.documentElement.dataset.theme = "dark";
  const content = page === "landing" ? <Landing /> : page === "compare" ? <Compare /> : <Changelog />;
  return render(<Page page={page}>{content}</Page>);
}

describe.each(["en", "ru"] as const)("pages in %s", (lang) => {
  const m = lang === "en" ? en : ru;

  it("renders the landing page with a download link per package", () => {
    renderPage(lang, "landing");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain(m.hero.title);
    for (const download of Object.values(release.downloads)) {
      if (download) expect(document.querySelector(`a[href="${download.url}"]`)).not.toBeNull();
    }
    expect(screen.getAllByText(release.intellijRepositoryUrl).length).toBeGreaterThan(0);
    expect(document.querySelector('a[href="https://coddy.dev"]')).not.toBeNull();
    expect(document.title).toBe(m.meta.landing.title);
  });

  it("renders every changelog version", () => {
    renderPage(lang, "changelog");
    for (const version of changelog.versions) {
      expect(screen.getByRole("heading", { level: 2, name: version.version })).toBeTruthy();
    }
  });

  it("renders one row per harness in every comparison table", () => {
    renderPage(lang, "compare");
    const tables = document.querySelectorAll("table.compare-table");
    expect(tables.length).toBe(compare.tables.length);
    tables.forEach((table) => {
      expect(table.querySelectorAll("tbody tr").length).toBe(compare.harnesses.length);
    });
  });
});

describe("header controls", () => {
  it("switch the language and the theme and remember both", () => {
    renderPage("en", "landing");
    fireEvent.click(screen.getByRole("button", { name: "RU" }));
    expect(document.documentElement.lang).toBe("ru");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain(ru.hero.title);
    expect(localStorage.getItem("foxxy-site-lang")).toBe("ru");

    fireEvent.click(screen.getByRole("button", { name: ru.nav.toLight }));
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(localStorage.getItem("foxxy-site-theme")).toBe("light");
  });
});
