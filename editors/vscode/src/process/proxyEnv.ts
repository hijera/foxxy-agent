export type ProxyEnv = Partial<Record<(typeof PROXY_ENV_KEYS)[number] | (typeof NO_PROXY_ENV_KEYS)[number], string>>;

const PROXY_ENV_KEYS = [
  "HTTP_PROXY",
  "HTTPS_PROXY",
  "ALL_PROXY",
  "http_proxy",
  "https_proxy",
  "all_proxy",
] as const;

const NO_PROXY_ENV_KEYS = ["NO_PROXY", "no_proxy"] as const;

/** Converts VS Code's `http.proxy` setting into proxy env vars for the child process. */
export function proxyEnvFromSetting(proxySetting: string | undefined): ProxyEnv {
  return proxyEnvFrom(proxySetting, []);
}

/**
 * Builds proxy env vars from VS Code's `http.proxy` + `http.noProxy` settings.
 * `NO_PROXY`/`no_proxy` is only emitted alongside a proxy URL (matches the IntelliJ side),
 * since it is meaningless without one.
 */
export function proxyEnvFrom(proxySetting: string | undefined, noProxy: readonly string[]): ProxyEnv {
  const proxyUrl = normalizeProxySetting(proxySetting);
  if (!proxyUrl) return {};
  const env: ProxyEnv = {};
  for (const key of PROXY_ENV_KEYS) env[key] = proxyUrl;
  const noProxyValue = normalizeNoProxy(noProxy);
  if (noProxyValue) {
    for (const key of NO_PROXY_ENV_KEYS) env[key] = noProxyValue;
  }
  return env;
}

/** Normalizes VS Code's `http.noProxy` array into a comma-joined `NO_PROXY` value. */
function normalizeNoProxy(noProxy: readonly string[]): string | null {
  const entries = noProxy.map((e) => e.trim()).filter((e) => e.length > 0);
  return entries.length ? entries.join(",") : null;
}

/** Merges proxy variables into a process environment without mutating the caller's object. */
export function withProxyEnv(base: NodeJS.ProcessEnv, proxyEnv: ProxyEnv): NodeJS.ProcessEnv {
  const out: NodeJS.ProcessEnv = { ...base };
  for (const [key, value] of Object.entries(proxyEnv)) {
    if (value) out[key] = value;
  }
  return out;
}

function normalizeProxySetting(proxySetting: string | undefined): string | null {
  const raw = proxySetting?.trim();
  if (!raw) return null;

  const candidate = /^[a-z][a-z\d+.-]*:\/\//i.test(raw) ? raw : `http://${raw}`;
  try {
    const url = new URL(candidate);
    if (!url.hostname) return null;
    if (url.protocol !== "http:" && url.protocol !== "https:") return null;
    return url.toString();
  } catch {
    return null;
  }
}
