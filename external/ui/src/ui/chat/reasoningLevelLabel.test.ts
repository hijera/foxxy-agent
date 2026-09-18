import { afterEach, describe, expect, it } from "vitest";
import { setLocale } from "../i18n/i18n";
import { reasoningLevelLabel } from "./reasoningLevelLabel";

afterEach(() => setLocale("en"));

describe("reasoningLevelLabel", () => {
  it("names every level the backend offers in the UI language", () => {
    setLocale("ru");
    expect(
      ["none", "minimal", "low", "medium", "high", "xhigh", "max"].map(
        reasoningLevelLabel,
      ),
    ).toEqual([
      "Без рассуждения",
      "Минимальный",
      "Низкий",
      "Средний",
      "Высокий",
      "Очень высокий",
      "Максимальный",
    ]);
  });

  it("keeps the English names readable", () => {
    expect(reasoningLevelLabel("medium")).toBe("Medium");
    expect(reasoningLevelLabel("xhigh")).toBe("Extra high");
  });

  // reasoning_levels is free-form in the config, so an id nobody translated is
  // still shown rather than dropped.
  it("shows an unknown level as its capitalized id", () => {
    setLocale("ru");
    expect(reasoningLevelLabel("turbo")).toBe("Turbo");
  });
});
