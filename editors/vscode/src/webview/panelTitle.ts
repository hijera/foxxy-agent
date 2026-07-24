/** Naming for the FoxxyCode panel chrome.
 *
 *  Both surfaces show the plugin version next to the product name so a bug report
 *  can be pinned to a build:
 *   - the sidebar view keeps its `%view.name%` title and puts the version in
 *     `WebviewView.description`, which VS Code renders dimmed right after it;
 *   - the editor-area panel has no description slot, so the version goes into the
 *     tab title itself.
 *  The version comes from the extension manifest (`extension.packageJSON.version`).
 */

export const PANEL_PRODUCT_NAME = "FoxxyCode";

/** Editor-panel tab title, e.g. `FoxxyCode 0.1.6`. Falls back to the bare name. */
export function formatPanelTitle(version: string | undefined | null): string {
  const v = (version || "").trim();
  return v === "" ? PANEL_PRODUCT_NAME : `${PANEL_PRODUCT_NAME} ${v}`;
}

/** Sidebar view description (rendered after the view name), or "" when unknown. */
export function formatViewDescription(
  version: string | undefined | null,
): string {
  return (version || "").trim();
}
