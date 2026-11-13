import { environmentKey, getEnv } from "../env/remoteEnv";

function key(appId: string): string {
  return `miniapp-run:${environmentKey(getEnv())}:${appId}`;
}
export function rememberMiniAppRun(appId: string, id: string): void {
  try {
    window.sessionStorage.setItem(key(appId), id);
  } catch {
    /* Storage may be disabled. */
  }
}
export function savedMiniAppRun(appId: string): string | null {
  try {
    return window.sessionStorage.getItem(key(appId));
  } catch {
    return null;
  }
}
export function forgetMiniAppRun(appId: string): void {
  try {
    window.sessionStorage.removeItem(key(appId));
  } catch {
    /* Storage may be disabled. */
  }
}
