// Reachability of the *active* environment (issue #60). A single shared monitor probes the
// selected remote on load, on an interval, and on window focus, so a down / misauthenticated
// remote is visible at a glance (composer chip dot + a banner) instead of the app silently
// rendering empty against a dead backend. Local is always "up".

import { useEffect } from "react";
import { useSyncExternalStore } from "react";
import { getEnv, probeRemote, type FoxxyCodeEnv } from "./remoteEnv";

export type EnvHealth = "checking" | "up" | "down";

const PROBE_INTERVAL_MS = 30_000;
const PROBE_TIMEOUT_MS = 6_000;

let health: EnvHealth = "up";
let started = false;
const listeners = new Set<() => void>();

function set(h: EnvHealth): void {
  if (h !== health) {
    health = h;
    listeners.forEach((cb) => cb());
  }
}

/** probeEnvHealth resolves "up" when the environment answers `GET /v1/models` (authorized), else
 * "down" (offline, DNS/TLS, refused, CORS-blocked, or 401/403). Local is always "up".
 *
 * A swarm relay is the one remote that answers neither: it serves no /v1 at all, only
 * /swarm/*. It still has to be added as a remote to be given its bearer token, so a 404
 * from /v1/models is followed by one ask of `GET /swarm/info` before calling it down -
 * otherwise every relay sits behind a permanent "unreachable" banner while its screen
 * works. A 401/403 stays down either way: that is a wrong token, not a relay. */
export async function probeEnvHealth(env: FoxxyCodeEnv): Promise<EnvHealth> {
  if (env.mode !== "remote") return "up";
  const headers = env.token ? { Authorization: "Bearer " + env.token } : {};
  try {
    const res = await probeRemote(
      env.baseUrl + "/v1/models",
      headers,
      PROBE_TIMEOUT_MS,
    );
    if (res.ok) return "up";
    if (res.status !== 404) return "down";
  } catch {
    return "down";
  }
  try {
    const relay = await probeRemote(
      env.baseUrl + "/swarm/info",
      headers,
      PROBE_TIMEOUT_MS,
    );
    return relay.ok ? "up" : "down";
  } catch {
    return "down";
  }
}

// One probe at a time: the interval and the focus event can fire while a probe
// is still waiting, and two interleaved runs would publish "checking" after a
// result that already arrived. A tick that finds a probe in flight joins it.
let inFlight: Promise<void> | null = null;

function tick(): Promise<void> {
  if (!inFlight) {
    inFlight = runTick().finally(() => {
      inFlight = null;
    });
  }
  return inFlight;
}

async function runTick(): Promise<void> {
  const env = getEnv();
  if (env.mode !== "remote") {
    set("up");
    return;
  }
  if (health !== "up") set("checking");
  set(await probeEnvHealth(env));
}

/** startActiveHealthMonitor begins probing the active environment (idempotent). */
export function startActiveHealthMonitor(): void {
  if (started || typeof window === "undefined") return;
  started = true;
  health = getEnv().mode === "remote" ? "checking" : "up";
  void tick();
  window.setInterval(() => void tick(), PROBE_INTERVAL_MS);
  window.addEventListener("focus", () => void tick());
}

export function subscribeHealth(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}
export function snapshotHealth(): EnvHealth {
  return health;
}

/** useActiveEnvHealth returns the live health of the active environment for React components. */
export function useActiveEnvHealth(): EnvHealth {
  useEffect(() => startActiveHealthMonitor(), []);
  return useSyncExternalStore(subscribeHealth, snapshotHealth, snapshotHealth);
}
