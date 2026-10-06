export type OnboardingStatus = {
  first_run: boolean;
  has_config: boolean;
  has_providers: boolean;
  has_models: boolean;
  has_agent_model: boolean;
  /** The provider behind agent.model has some credential source (key, command, env var, stored login). */
  has_agent_credentials: boolean;
  /** Providers with no credential source at all. Informational; the picker is not gated on it. */
  missing_api_keys: string[];
  suggested_defaults?: {
    provider_name?: string;
    provider_type?: string;
    model?: string;
    max_tokens?: number;
    temperature?: number;
  };
};

export async function fetchOnboardingStatus(): Promise<OnboardingStatus | null> {
  try {
    const res = await fetch("/foxxycode/onboarding/status");
    if (!res.ok) return null;
    return (await res.json()) as OnboardingStatus;
  } catch {
    return null;
  }
}

export function shouldShowOnboarding(status: OnboardingStatus | null): boolean {
  if (!status) return false;
  if (status.first_run) return true;
  if (!status.has_providers) return true;
  if (!status.has_models) return true;
  if (!status.has_agent_model) return true;
  // Only the agent's own provider matters. A second provider left without a key
  // (say, an openai row next to a keyed neuraldeep one) is listed in
  // missing_api_keys for information but must not pull the picker up. A strict
  // false check keeps a bundle ahead of its backend (field absent) from opening
  // the picker on every load.
  if (status.has_agent_credentials === false) return true;
  return false;
}
