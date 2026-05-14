const PROVIDER_NAME_RE = /^[a-zA-Z][a-zA-Z0-9_-]*$/;

/** Mirrors internal/config.ProviderAPIKeyEnvVarName for UI placeholder text. */
export function providerAPIKeyEnvVarName(providerName: string): string {
  const s = providerName.trim();
  if (!PROVIDER_NAME_RE.test(s)) {
    return "";
  }
  return s.replace(/-/g, "_").toUpperCase() + "_API_KEY";
}

/** Placeholder for the provider api_key field; updates with the sibling provider name. */
export function providerApiKeyFieldPlaceholder(providerName: string): string {
  const env = providerAPIKeyEnvVarName(providerName);
  if (env) {
    // Build "${ENV}" with string concat so minifiers never treat "${...}" as a template slot.
    return (
      "If empty, reads from $" +
      "{" +
      env +
      "} at run time, or set a literal key (YAML may use $" +
      "{VAR} at load)"
    );
  }
  return (
    "Provider id must start with a letter. When the id is valid, leave empty " +
    "to read the NAME_API_KEY variable (NAME is uppercase, hyphens become underscores)."
  );
}
