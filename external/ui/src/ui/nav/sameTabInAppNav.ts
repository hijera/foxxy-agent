import type { MouseEvent } from "react";

/**
 * Run in-app navigation on plain left click only. Middle-click, modified
 * clicks, and non-primary buttons keep default so the browser can open a new tab.
 */
export function sameTabInAppNavClick(
  ev: MouseEvent<HTMLAnchorElement>,
  action: () => void,
): void {
  if (ev.defaultPrevented) {
    return;
  }
  if (ev.button !== 0) {
    return;
  }
  if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) {
    return;
  }
  ev.preventDefault();
  action();
}
