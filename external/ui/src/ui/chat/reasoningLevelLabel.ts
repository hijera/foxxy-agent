import { t } from "../i18n/i18n";

/**
 * Message keys for the reasoning level names the backend offers
 * (`internal/config/reasoning.go`: minimal/low/medium/high, `none` on Codex, whose
 * top tier is `xhigh`; `max` is what an operator may type into reasoning_levels).
 */
const LEVEL_KEYS: Record<string, string> = {
  none: "composer.reasoningLevelNone",
  minimal: "composer.reasoningLevelMinimal",
  low: "composer.reasoningLevelLow",
  medium: "composer.reasoningLevelMedium",
  high: "composer.reasoningLevelHigh",
  xhigh: "composer.reasoningLevelXhigh",
  max: "composer.reasoningLevelMax",
};

/**
 * Localizes a reasoning level for the composer pill and its menu.
 *
 * The level is a config id sent back verbatim as `metadata.reasoning`, so only
 * its display is translated. An id the UI has no name for (levels are free-form
 * in `models[].reasoning_levels`) is shown as the id itself, capitalized, the
 * way a model id is shown as its name.
 */
export function reasoningLevelLabel(level: string): string {
  const id = (level || "").trim();
  const key = LEVEL_KEYS[id.toLowerCase()];
  if (key) {
    return t(key);
  }
  return id.slice(0, 1).toUpperCase() + id.slice(1);
}
