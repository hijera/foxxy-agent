export type SwarmLayoutMode = "tree" | "graph";

export const SWARM_LAYOUT_STORAGE_KEY = "foxxycode_swarm_layout";

type LayoutStorage = Pick<Storage, "getItem" | "setItem">;

export function readSwarmLayoutMode(storage?: LayoutStorage): SwarmLayoutMode {
  try {
    const resolvedStorage = storage ?? globalThis.localStorage;
    const value = resolvedStorage?.getItem(SWARM_LAYOUT_STORAGE_KEY);
    // "star" is the value the retired mode wrote; the selection it stood for
    // is the graph, so a preference saved before the rename still applies.
    return value === "graph" || value === "star" ? "graph" : "tree";
  } catch {
    return "tree";
  }
}

export function writeSwarmLayoutMode(
  mode: SwarmLayoutMode,
  storage?: LayoutStorage,
): void {
  try {
    const resolvedStorage = storage ?? globalThis.localStorage;
    // Anything but an explicit tree picks the graph: a caller still passing
    // the retired "star" through an unchecked cast lands on its new name.
    resolvedStorage?.setItem(
      SWARM_LAYOUT_STORAGE_KEY,
      mode === "tree" ? "tree" : "graph",
    );
  } catch {
    // Layout preferences are optional when storage is blocked or full.
  }
}
