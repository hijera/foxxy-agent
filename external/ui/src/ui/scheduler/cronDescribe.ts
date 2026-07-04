import cronstrue from "cronstrue";
import { getLocale, t } from "../i18n/i18n";

/**
 * Human-readable description of a 5-field cron (UTC on server).
 * Returns null when spec is empty or invalid.
 */
export function describeCronScheduleUTC(spec: string): string | null {
  const s = spec.trim();
  if (!s) {
    return null;
  }
  try {
    return cronstrue.toString(s, {
      use24HourTimeFormat: true,
      verbose: true,
      locale: getLocale(),
    });
  } catch {
    return null;
  }
}

export function describeCronScheduleOrError(spec: string): {
  ok: true;
  text: string;
} | { ok: false; error: string } {
  const s = spec.trim();
  if (!s) {
    return { ok: false, error: t("scheduler.cron.empty") };
  }
  try {
    const text = cronstrue.toString(s, {
      use24HourTimeFormat: true,
      verbose: true,
      locale: getLocale(),
    });
    return { ok: true, text };
  } catch {
    return { ok: false, error: t("scheduler.cron.invalid") };
  }
}
