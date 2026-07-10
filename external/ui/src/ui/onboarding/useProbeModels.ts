import { useCallback, useRef, useState } from "react";
import type { FetchedModel } from "../settings/useProviderModels";

export type ProbeModelsInput = {
  type: "openai" | "anthropic" | "neuraldeep";
  api_base: string;
  api_key: string;
};

type ProbeModelsResponse = {
  ok?: boolean;
  error?: string;
  models?: FetchedModel[];
};

/**
 * useProbeModels fetches the model list for a provider that is not saved in the
 * config yet (onboarding) via POST /foxxycode/providers/models-probe, sending the
 * credentials in the body. On failure it surfaces an error and an empty list so
 * the dialog falls back to manual model entry. Overlapping requests are guarded
 * by a sequence counter: only the latest probe's result is applied.
 */
export function useProbeModels() {
  const [loading, setLoading] = useState(false);
  const [models, setModels] = useState<FetchedModel[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [fetched, setFetched] = useState(false);
  const seq = useRef(0);

  const probe = useCallback(async (input: ProbeModelsInput) => {
    const mySeq = ++seq.current;
    setLoading(true);
    setError(null);
    try {
      const res = await fetch("/foxxycode/providers/models-probe", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      });
      const data = (await res.json().catch(() => ({}))) as ProbeModelsResponse;
      if (mySeq !== seq.current) {
        return;
      }
      if (!res.ok || !data.ok) {
        setModels([]);
        setError(data?.error || `HTTP ${res.status}`);
      } else {
        setModels(data.models ?? []);
      }
    } catch (e) {
      if (mySeq !== seq.current) {
        return;
      }
      setModels([]);
      setError(e instanceof Error ? e.message : "request failed");
    } finally {
      if (mySeq === seq.current) {
        setLoading(false);
        setFetched(true);
      }
    }
  }, []);

  const reset = useCallback(() => {
    seq.current++;
    setModels([]);
    setError(null);
    setFetched(false);
    setLoading(false);
  }, []);

  return { loading, models, error, fetched, probe, reset };
}
