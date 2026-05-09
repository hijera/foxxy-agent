import { expect, test } from 'vitest';
import { CODDY_NAV_RAIL_COOKIE, readNavRailCookie, writeNavRailCookie } from './navRailCookie';

test('write then read nav rail cookie mode', () => {
  document.cookie = `${CODDY_NAV_RAIL_COOKIE}=; Max-Age=0; Path=/`;
  Object.defineProperty(window, 'location', { value: new URL('http://127.0.0.1:5173/'), configurable: true });
  writeNavRailCookie('wide');
  expect(readNavRailCookie()).toBe('wide');
});
