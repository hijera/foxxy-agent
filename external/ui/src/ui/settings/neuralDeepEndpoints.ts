// Official deployments, mirroring neuralDeepEndpoints in internal/llm/neuraldeep_auth.go.
export const NEURALDEEP_API_BASE_OPTIONS = [
  {
    value: "https://api.neuraldeep.ru/v1",
    hub: "https://hub.neuraldeep.ru",
    labelKey: "settings.neuralDeepApiBase.optionRu",
  },
  {
    value: "https://api.neuraldeep.tech/v1",
    hub: "https://hub.neuraldeep.tech",
    labelKey: "settings.neuralDeepApiBase.optionTech",
  },
] as const;

export const NEURALDEEP_DEFAULT_API_BASE = NEURALDEEP_API_BASE_OPTIONS[0].value;

/** Canonical spelling of a stored api_base, or "" for an unknown endpoint. */
export function matchNeuralDeepAPIBase(value: unknown): string {
  const want = String(value ?? "")
    .trim()
    .replace(/\/+$/, "")
    .toLowerCase();
  return (
    NEURALDEEP_API_BASE_OPTIONS.find((o) => o.value.toLowerCase() === want)
      ?.value ?? ""
  );
}

export function neuralDeepHubFor(apiBase: string): string {
  return (
    NEURALDEEP_API_BASE_OPTIONS.find((o) => o.value === apiBase)?.hub ??
    NEURALDEEP_API_BASE_OPTIONS[0].hub
  );
}
