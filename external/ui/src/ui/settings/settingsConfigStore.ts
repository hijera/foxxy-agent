// The copy of the server's configuration that the Settings drawer edits, kept for
// the life of the page.
//
// Settings is mounted only while it is open. Reading /foxxycode/config/schema and
// /foxxycode/config on every mount drew the drawer empty first - only the tabs that
// need no schema, Appearance in place of the tab a deep link named - and rebuilt
// it when the answers came (issue #359). The two answers are read once and kept
// here, and an open draws from this copy at once. The copy follows the server
// through the events stream: every config_reloaded, and every reconnect of the
// stream (an event may have been missed while it was down), reads it again in the
// background (noteSettingsConfigReloaded, called from App.tsx).
//
// Changing the environment reloads the page, so a copy never outlives the server
// it came from.

import type { JsonSchema } from "./SchemaForm";
import { translate } from "../i18n/i18n";

export type SettingsConfigCopy = {
  schema: JsonSchema | null;
  config: Record<string, unknown> | null;
  /**
   * Why the first read failed. A copy in hand is kept over a failed read, so
   * this is set only while there is none.
   */
  error: string | null;
};

export type SettingsConfigRead =
  | { ok: true; copy: SettingsConfigCopy }
  | { ok: false; error: string };

const empty: SettingsConfigCopy = { schema: null, config: null, error: null };

let current: SettingsConfigCopy = empty;
const listeners = new Set<() => void>();
/** The first read, shared by every open that comes before it lands. */
let firstRead: Promise<SettingsConfigCopy> | null = null;
/** Reads are numbered: an answer older than the one on hand is dropped. */
let issued = 0;
let applied = 0;
/**
 * The newest read that failed. When it is newer than the copy on hand, the copy
 * may predate a reload the failed read was asked for, and the next open reads
 * again instead of trusting it.
 */
let failedAfter = 0;
/** Bumped by a reset, so an answer to a read from before it is dropped too. */
let generation = 0;

export function subscribeSettingsConfig(cb: () => void): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

export function snapshotSettingsConfig(): SettingsConfigCopy {
  return current;
}

function publish(next: SettingsConfigCopy): void {
  current = next;
  listeners.forEach((cb) => cb());
}

/** resetSettingsConfigForTests forgets the copy and every read on its way. */
export function resetSettingsConfigForTests(): void {
  generation++;
  firstRead = null;
  issued = 0;
  applied = 0;
  failedAfter = 0;
  publish(empty);
}

async function readJSON<T>(
  path: string,
): Promise<{ ok: true; data: T } | { ok: false; error: string }> {
  try {
    const res = await fetch(path);
    if (!res.ok) {
      return { ok: false, error: `${res.status}` };
    }
    return { ok: true, data: (await res.json()) as T };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "request" };
  }
}

async function read(): Promise<SettingsConfigRead> {
  const n = ++issued;
  const gen = generation;
  // A retry after a failed first read shows the tab loading again, not the
  // error of the attempt before.
  if (current.config === null && current.error !== null) {
    publish({ ...current, error: null });
  }
  const [s, c] = await Promise.all([
    readJSON<JsonSchema>("/foxxycode/config/schema"),
    readJSON<Record<string, unknown>>("/foxxycode/config"),
  ]);
  if (gen !== generation) {
    return { ok: false, error: "superseded" };
  }
  if (!s.ok || !c.ok) {
    const error = !s.ok
      ? s.error || translate("settings.error.schemaLoadFailed")
      : (!c.ok && c.error) || translate("settings.error.configLoadFailed");
    // A copy in hand stays, and the next open asks again. Without one the
    // drawer says why it has nothing to show.
    failedAfter = Math.max(failedAfter, n);
    if (current.config === null && n > applied) {
      publish({ ...current, error });
    }
    return { ok: false, error };
  }
  if (n > applied) {
    applied = n;
    publish({ schema: s.data, config: c.data, error: null });
  }
  return { ok: true, copy: current };
}

/**
 * ensureSettingsConfig reads the schema and the config unless a copy is held:
 * the first open reads them, every later one draws from the copy. Opens that
 * come while the first read is on its way share it.
 */
export function ensureSettingsConfig(): Promise<SettingsConfigCopy> {
  if (
    current.schema !== null &&
    current.config !== null &&
    failedAfter <= applied
  ) {
    return Promise.resolve(current);
  }
  if (firstRead === null) {
    const pending: Promise<SettingsConfigCopy> = read()
      .then(() => current)
      .finally(() => {
        if (firstRead === pending) {
          firstRead = null;
        }
      });
    firstRead = pending;
  }
  return firstRead;
}

/**
 * withoutReplacedRevision drops the name (revision) of the configuration a
 * document was read from. A save overwrote that one, so its name must not
 * reach the next save: PUT /foxxycode/config measures "untouched" against the
 * configuration the body's revision names (restoreUntouched keeps what the
 * file says for a value sent back as it was served), and against a replaced
 * document a change-and-revert to one of its values looks untouched, so the
 * revert is lost. A document with no name is measured against the
 * configuration live when the PUT arrives - which, right after a save, is what
 * the save left in the file - until a read brings the revision of what is on
 * disk now.
 */
export function withoutReplacedRevision(
  config: Record<string, unknown>,
): Record<string, unknown> {
  if (!("revision" in config)) {
    return config;
  }
  const saved = { ...config };
  delete saved.revision;
  return saved;
}

/**
 * noteSettingsConfigSaved takes the document a save returned as the copy, so
 * an open that comes before the read after the save lands - or after that read
 * failed - draws what was saved rather than what the save replaced (a save made
 * from that older copy would put the replaced values back). Reads issued before
 * the save are older than it and are dropped. The document it returns and
 * publishes keeps the returned revision when authoritative is true. Older
 * servers return only ok, so their optimistic copy drops the replaced revision
 * until a read supplies the actual saved document (see withoutReplacedRevision).
 */
export function noteSettingsConfigSaved(
  config: Record<string, unknown>,
  authoritative = false,
): Record<string, unknown> {
  applied = issued;
  // New servers return the merged document and its revision atomically with
  // the save. Only legacy responses need the unversioned optimistic copy.
  const saved = authoritative ? config : withoutReplacedRevision(config);
  publish({ schema: current.schema, config: saved, error: null });
  return saved;
}

/** refreshSettingsConfig reads the schema and the config again now. */
export function refreshSettingsConfig(): Promise<SettingsConfigRead> {
  return read();
}

/**
 * noteSettingsConfigReloaded is told that the server swapped its
 * configuration, or that the events stream came back and may have missed it. A
 * copy that is held or on its way, or a first read that failed, is read again
 * in the background; nothing is read for a Settings drawer never opened.
 */
export function noteSettingsConfigReloaded(): void {
  if (current.config !== null || firstRead !== null || current.error !== null) {
    void read();
  }
}
