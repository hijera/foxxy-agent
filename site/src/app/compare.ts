import data from "../data/compare.json";
import type { Lang, Localized } from "./prefs";

export type Status = "yes" | "partial" | "no" | "unknown" | "na" | "info";
export const STATUSES: Status[] = ["yes", "partial", "no", "unknown", "na", "info"];

/**
 * One table cell. `s` is the verdict; the text is either shared by both languages (`t`, for
 * names such as "Go" or "MIT") or given per language (`en` and `ru`). A cell with no text shows
 * the localized word for its verdict.
 */
export interface Cell {
  s: Status;
  t?: string;
  en?: string;
  ru?: string;
}

export interface Harness {
  id: string;
  name: string;
  url: string;
  role: "self" | "upstream" | "other";
  note?: Localized;
}

export interface Column {
  id: string;
  label: Localized;
}

export interface CompareTable {
  id: string;
  title: Localized;
  lead: Localized;
  columns: Column[];
  rows: Record<string, Record<string, Cell>>;
}

export interface Source {
  label: string;
  url: string;
}

export interface CompareData {
  compiled: string;
  harnesses: Harness[];
  tables: CompareTable[];
  sources: Source[];
}

export const compare = data as unknown as CompareData;

export function cellText(cell: Cell, lang: Lang): string | null {
  return cell.t ?? cell[lang] ?? null;
}
