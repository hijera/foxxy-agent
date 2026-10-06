import { afterEach, describe, expect, it, vi } from "vitest";
import { BOOT_SCRIPT, LANG_KEY, THEME_KEY } from "../scripts/lib/boot.mjs";

function boot({
  search = "",
  languages = ["en-US"],
  prefersLight = false,
}: {
  search?: string;
  languages?: string[];
  prefersLight?: boolean;
}) {
  window.history.replaceState(null, "", `/foxxy-agent/${search}`);
  vi.spyOn(navigator, "languages", "get").mockReturnValue(languages);
  window.matchMedia = vi.fn().mockReturnValue({ matches: prefersLight }) as unknown as typeof window.matchMedia;
  new Function(BOOT_SCRIPT)();
  return { lang: document.documentElement.lang, theme: document.documentElement.dataset.theme };
}

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("head boot script", () => {
  it("follows the browser language and the system theme on a first visit", () => {
    expect(boot({ languages: ["ru-RU", "en"], prefersLight: true })).toEqual({ lang: "ru", theme: "light" });
    expect(boot({ languages: ["de-DE", "fr"] })).toEqual({ lang: "en", theme: "dark" });
    expect(boot({ languages: ["de-DE", "en-GB"] }).lang).toBe("en");
  });

  it("prefers a stored choice over the browser", () => {
    localStorage.setItem(LANG_KEY, "en");
    localStorage.setItem(THEME_KEY, "dark");
    expect(boot({ languages: ["ru-RU"], prefersLight: true })).toEqual({ lang: "en", theme: "dark" });
  });

  it("lets ?lang= win and remembers it", () => {
    localStorage.setItem(LANG_KEY, "en");
    expect(boot({ search: "?lang=ru" }).lang).toBe("ru");
    expect(localStorage.getItem(LANG_KEY)).toBe("ru");
    expect(boot({ search: "?lang=de" }).lang).toBe("ru");
  });

  it("ignores values it does not know", () => {
    localStorage.setItem(THEME_KEY, "solarized");
    expect(boot({ prefersLight: true }).theme).toBe("light");
  });
});
