// @ts-check
// The inline script every page runs in <head>, before the stylesheet: it settles the language and
// the theme on <html> so the first paint already has the right colors and no English flashes on a
// Russian visitor. React reads both back from <html> (src/app/prefs.ts), so the two never disagree.
//
// Language: ?lang=ru|en, then the stored choice, then the browser languages, then English.
// Theme: the stored choice, then prefers-color-scheme.

export const LANG_KEY = "foxxy-site-lang";
export const THEME_KEY = "foxxy-site-theme";

export const BOOT_SCRIPT = `(function () {
  var d = document.documentElement, lang = null, theme = null, s = null;
  try { s = window.localStorage; } catch (e) {}
  try {
    var q = new URLSearchParams(location.search).get("lang");
    if (q === "ru" || q === "en") { lang = q; if (s) s.setItem("${LANG_KEY}", q); }
  } catch (e) {}
  if (!lang && s) { var l = s.getItem("${LANG_KEY}"); if (l === "ru" || l === "en") lang = l; }
  if (!lang) {
    var langs = navigator.languages || [navigator.language || ""];
    for (var i = 0; i < langs.length; i++) {
      var p = String(langs[i]).toLowerCase().split("-")[0];
      if (p === "ru" || p === "en") { lang = p; break; }
    }
  }
  if (s) { var t = s.getItem("${THEME_KEY}"); if (t === "dark" || t === "light") theme = t; }
  if (!theme) theme = window.matchMedia && matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  d.lang = lang || "en";
  d.dataset.theme = theme;
  d.style.colorScheme = theme;
})();`;
