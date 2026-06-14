/**
 * Pure helpers for the composer model selector menu (`metadata.model` backends).
 *
 * Backends are YAML ids in `vendor/model` form (for example
 * `opencode-go/deepseek-v4-pro`). When many backends are configured the menu
 * gains a filter input and groups rows under their vendor; these helpers keep
 * that logic testable and out of the React component.
 */

/** Above this many backends the menu shows a filter input. */
export const LLM_MENU_FILTER_THRESHOLD = 5;

/** Vendor prefix (before the first `/`); `""` when the id has no slash. */
export function llmVendorOf(id: string): string {
  const m = id || "";
  const i = m.indexOf("/");
  return i > 0 ? m.slice(0, i) : "";
}

/** Model name (after the first `/`); the whole id when there is no slash. */
export function llmModelNameOf(id: string): string {
  const m = id || "";
  const i = m.indexOf("/");
  return i >= 0 && i < m.length - 1 ? m.slice(i + 1) : m;
}

/** Whether the filter input should render for a list of this size. */
export function shouldShowLlmFilter(count: number): boolean {
  return count > LLM_MENU_FILTER_THRESHOLD;
}

/** Number of distinct vendor buckets (slashless ids share the `""` bucket). */
export function llmMenuVendorCount(ids: readonly string[]): number {
  const seen = new Set<string>();
  for (const id of ids) {
    seen.add(llmVendorOf(id));
  }
  return seen.size;
}

/** Group rows under vendor headers only when more than one vendor is present. */
export function shouldGroupLlmModels(ids: readonly string[]): boolean {
  return llmMenuVendorCount(ids) > 1;
}

/**
 * Case-insensitive substring filter over the full id, vendor, and model name.
 * An empty or whitespace-only query returns the list unchanged (original order).
 */
export function filterLlmModels(
  ids: readonly string[],
  query: string,
): string[] {
  const q = (query || "").trim().toLowerCase();
  if (!q) {
    return [...ids];
  }
  return ids.filter((id) => {
    const full = id.toLowerCase();
    const vendor = llmVendorOf(id).toLowerCase();
    const name = llmModelNameOf(id).toLowerCase();
    return full.includes(q) || vendor.includes(q) || name.includes(q);
  });
}

export type LlmModelGroup = {
  /** Vendor prefix; `""` for slashless ids. */
  vendor: string;
  /** Full backend ids in this vendor, original relative order preserved. */
  models: string[];
};

/** Bucket ids by vendor, preserving first-seen vendor order and member order. */
export function groupLlmModelsByVendor(
  ids: readonly string[],
): LlmModelGroup[] {
  const groups: LlmModelGroup[] = [];
  const byVendor = new Map<string, LlmModelGroup>();
  for (const id of ids) {
    const vendor = llmVendorOf(id);
    let group = byVendor.get(vendor);
    if (!group) {
      group = { vendor, models: [] };
      byVendor.set(vendor, group);
      groups.push(group);
    }
    group.models.push(id);
  }
  return groups;
}
