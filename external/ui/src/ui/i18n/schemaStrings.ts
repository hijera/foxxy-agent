import { getLocale } from "./i18n";
import { schemaEnumLabelRu, schemaTextRu } from "./messages/schema.ru";

/**
 * Translate a config JSON Schema string (a field `title` or `description` from
 * the Go backend). English is canonical; for `ru` we look up an overlay entry and
 * fall back to the original English text when none exists. Returns "" for
 * undefined/empty so callers can render nothing.
 */
export function tSchemaText(text: string | undefined | null): string {
  if (text === undefined || text === null || text === "") {
    return "";
  }
  if (getLocale() === "ru") {
    return schemaTextRu[text] ?? text;
  }
  return text;
}

/**
 * Human-readable label for an enum token. The token stays the config value; this
 * only affects how the option is shown in a dropdown. Falls back to the token.
 */
export function tSchemaEnumLabel(token: string): string {
  if (getLocale() === "ru") {
    return schemaEnumLabelRu[token] ?? token;
  }
  return token;
}
