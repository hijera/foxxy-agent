// Desktop-shell detection for the embedded WebView2 launcher.
//
// The desktop launcher navigates the SPA to `…/?desktop=1#/chat`
// (see internal/desktop/start_url.go). Hash-route changes inside the SPA keep
// the query string, but to stay robust we latch the flag into sessionStorage on
// first load so `isDesktopShell()` works from anywhere without re-parsing.

const STORAGE_KEY = "foxxycode.desktop";

/** Parse `?desktop=1` from the current URL query. */
export function readDesktopFromUrl(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return /[?&]desktop=1(?:&|$)/.test(window.location.search);
}

/**
 * Latch the desktop flag from the URL into sessionStorage (once per tab) and
 * return whether we are running inside the desktop shell. Safe to call before
 * React mounts, mirroring bootstrapUiLocaleFromUrlOrCookie().
 */
export function bootstrapDesktopFlag(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  if (readDesktopFromUrl()) {
    try {
      window.sessionStorage.setItem(STORAGE_KEY, "1");
    } catch {
      // Ignore storage failures (private mode / disabled storage).
    }
    return true;
  }
  return isDesktopShell();
}

/** Whether the SPA is running inside the desktop shell (latched flag). */
export function isDesktopShell(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  if (readDesktopFromUrl()) {
    return true;
  }
  try {
    return window.sessionStorage.getItem(STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}
