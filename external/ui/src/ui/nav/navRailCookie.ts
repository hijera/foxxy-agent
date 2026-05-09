export const CODDY_NAV_RAIL_COOKIE = 'coddy_nav_rail';

const MAX_AGE_SECONDS = 365 * 24 * 60 * 60;

export type NavRailCookieMode = 'wide' | 'narrow';

export function readNavRailCookie(): NavRailCookieMode | null {
  if (typeof document === 'undefined') {
    return null;
  }
  const parts = document.cookie.split(';');
  for (const p of parts) {
    const s = p.trim();
    if (!s.startsWith(`${CODDY_NAV_RAIL_COOKIE}=`)) {
      continue;
    }
    const v = decodeURIComponent(s.slice(CODDY_NAV_RAIL_COOKIE.length + 1).trim());
    if (v === 'wide' || v === 'narrow') {
      return v;
    }
    return null;
  }
  return null;
}

export function writeNavRailCookie(mode: NavRailCookieMode): void {
  if (typeof document === 'undefined') {
    return;
  }
  const secure = typeof window !== 'undefined' && window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${CODDY_NAV_RAIL_COOKIE}=${encodeURIComponent(mode)}; Path=/; Max-Age=${MAX_AGE_SECONDS}; SameSite=Lax${secure}`;
}
