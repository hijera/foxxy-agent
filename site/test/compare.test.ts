import { describe, expect, it } from "vitest";
import { STATUSES, compare, type Cell } from "../src/app/compare";

const CYRILLIC = /[Ѐ-ӿ]/;

describe("comparison data", () => {
  const ids = compare.harnesses.map((h) => h.id);

  it("lists FoxxyCode once as this project and Coddy as upstream", () => {
    expect(new Set(ids).size).toBe(ids.length);
    expect(compare.harnesses.filter((h) => h.role === "self").map((h) => h.id)).toEqual(["foxxycode"]);
    expect(compare.harnesses.filter((h) => h.role === "upstream").map((h) => h.id)).toEqual(["coddy"]);
    expect(compare.harnesses[0]?.id).toBe("foxxycode");
    for (const h of compare.harnesses) expect(h.url, h.id).toMatch(/^https:\/\//);
  });

  it("has a cell for every harness in every column, and nothing extra", () => {
    for (const table of compare.tables) {
      expect(Object.keys(table.rows).sort(), table.id).toEqual([...ids].sort());
      const columns = table.columns.map((c) => c.id);
      for (const id of ids) {
        expect(Object.keys(table.rows[id]!).sort(), `${table.id}.${id}`).toEqual([...columns].sort());
      }
    }
  });

  it("uses known verdicts and gives every text in both languages", () => {
    const check = (where: string, cell: Cell) => {
      expect(STATUSES, where).toContain(cell.s);
      if (cell.t !== undefined) {
        expect(cell.en ?? cell.ru, `${where}: shared text and per-language text at once`).toBeUndefined();
        expect(CYRILLIC.test(cell.t), `${where}: shared text must not be Russian`).toBe(false);
      } else if (cell.en !== undefined || cell.ru !== undefined) {
        expect(cell.en?.trim(), `${where}.en`).toBeTruthy();
        expect(cell.ru?.trim(), `${where}.ru`).toBeTruthy();
        expect(CYRILLIC.test(cell.en!), `${where}.en has Russian`).toBe(false);
      } else {
        expect(cell.s, `${where}: an info cell needs text`).not.toBe("info");
      }
    };
    for (const table of compare.tables) {
      expect(table.title.en && table.title.ru && table.lead.en && table.lead.ru, table.id).toBeTruthy();
      for (const col of table.columns) expect(col.label.en && col.label.ru, `${table.id}.${col.id}`).toBeTruthy();
      for (const [id, row] of Object.entries(table.rows)) {
        for (const [col, cell] of Object.entries(row)) check(`${table.id}.${id}.${col}`, cell);
      }
    }
  });

  it("puts the IDE and desktop table first and cites its sources", () => {
    expect(compare.tables[0]?.id).toBe("ide");
    expect(compare.compiled).toMatch(/^\d{4}-\d{2}$/);
    expect(compare.sources.length).toBeGreaterThan(10);
    expect(compare.sources.some((s) => s.url.startsWith("https://coddy.dev/compare"))).toBe(true);
  });

  it("claims every IDE capability for FoxxyCode", () => {
    const row = compare.tables.find((t) => t.id === "ide")!.rows.foxxycode!;
    for (const [col, cell] of Object.entries(row)) expect(cell.s, col).toBe("yes");
  });
});
