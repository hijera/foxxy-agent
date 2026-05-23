/** Default YAML backend for a new chat (cookie, then config default, then first row). */
export function pickDefaultLlmModelForNewChat(opts: {
  backends: readonly string[];
  cookie: string | null;
  defaultAgentModel: string;
}): string {
  const backends = opts.backends;
  const cookie = (opts.cookie || "").trim();
  if (cookie && backends.includes(cookie)) {
    return cookie;
  }
  const def = (opts.defaultAgentModel || "").trim();
  if (def && backends.includes(def)) {
    return def;
  }
  return backends[0]?.trim() || "";
}

/** YAML backend when opening an existing session (session wins over global cookie). */
export function pickLlmModelForOpenSession(opts: {
  backends: readonly string[];
  sessionModel: string | null | undefined;
  cookie: string | null;
  defaultAgentModel: string;
}): string {
  const sessionModel = (opts.sessionModel || "").trim();
  if (sessionModel && opts.backends.includes(sessionModel)) {
    return sessionModel;
  }
  return pickDefaultLlmModelForNewChat({
    backends: opts.backends,
    cookie: opts.cookie,
    defaultAgentModel: opts.defaultAgentModel,
  });
}
