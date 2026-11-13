import { localFetch } from "../env/remoteEnv";
import type {
  SwarmInfo,
  SwarmNode,
  SwarmSessionList,
  SwarmTopology,
} from "./types";

/**
 * A relay's API. By default it is reached through whatever fetch the app
 * already installed: the environment shim rewrites same-origin API paths to the
 * selected environment, which is the relay itself when the app is on it.
 *
 * Inside a node the environment is that node's mount, and the relay's own routes
 * are not under it. The map is still the relay's, so a caller names the relay
 * (`RelayTarget`) and the questions go straight to it with its client token -
 * which is how the map opens over a node without leaving it (issue #401).
 */
export type RelayTarget = { baseUrl: string; token: string };

/** An HTTP failure that kept its status, so callers can tell 401 from 503. */
export class SwarmHttpError extends Error {
  readonly status: number;
  constructor(path: string, status: number) {
    super(`${path}: ${status}`);
    this.name = "SwarmHttpError";
    this.status = status;
  }
}

async function getJSON<T>(
  path: string,
  signal?: AbortSignal,
  relay?: RelayTarget,
): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (relay?.token) {
    headers.Authorization = "Bearer " + relay.token;
  }
  const init: RequestInit = { headers };
  if (signal) {
    init.signal = signal;
  }
  const res = relay
    ? await localFetch(relay.baseUrl.replace(/\/+$/, "") + path, init)
    : await fetch(path, init);
  if (!res.ok) {
    throw new SwarmHttpError(path, res.status);
  }
  return (await res.json()) as T;
}

/**
 * Asks whether the current environment (or the named relay) is a relay.
 *
 * A relay serves no model catalog, so the usual probe reports it as down. This
 * one is public by design: a client has to be able to tell a relay from a plain
 * agent before it holds any credential.
 */
export async function probeSwarm(
  signal?: AbortSignal,
  relay?: RelayTarget,
): Promise<SwarmInfo | null> {
  try {
    const info = await getJSON<SwarmInfo>("/swarm/info", signal, relay);
    return info && info.swarm ? info : null;
  } catch {
    return null;
  }
}

export async function fetchNodes(
  signal?: AbortSignal,
  relay?: RelayTarget,
): Promise<SwarmNode[]> {
  const out = await getJSON<{ nodes: SwarmNode[] }>(
    "/swarm/nodes",
    signal,
    relay,
  );
  return out.nodes || [];
}

export async function fetchSwarmSessions(
  opts: { q?: string; node?: string; limit?: number } = {},
  signal?: AbortSignal,
  relay?: RelayTarget,
): Promise<SwarmSessionList> {
  const params = new URLSearchParams();
  params.set("limit", String(opts.limit ?? 100));
  params.set("include_activity", "true");
  if (opts.q) {
    params.set("q", opts.q);
  }
  if (opts.node) {
    params.set("node", opts.node);
  }
  const out = await getJSON<SwarmSessionList>(
    `/swarm/sessions?${params.toString()}`,
    signal,
    relay,
  );
  return {
    sessions: out.sessions || [],
    warnings: out.warnings || [],
    node_more: out.node_more || {},
    hasMore: !!out.hasMore,
  };
}

export async function fetchTopology(
  signal?: AbortSignal,
  relay?: RelayTarget,
): Promise<SwarmTopology> {
  return getJSON<SwarmTopology>("/swarm/topology", signal, relay);
}
