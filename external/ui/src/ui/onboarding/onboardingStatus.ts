export type OnboardingStatus = {
  first_run: boolean;
  has_config: boolean;
  has_providers: boolean;
  has_models: boolean;
  has_agent_model: boolean;
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
  if (status.missing_api_keys && status.missing_api_keys.length > 0) return true;
  return false;
}
