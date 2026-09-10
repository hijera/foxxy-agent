package llm

import "context"

// RawCompleter is implemented by providers that can run a plain text completion:
// the prompt goes to the model verbatim, with no chat template around it. It is
// what fill-in-the-middle prompts need, because their control tokens only work
// when they reach the model as tokens rather than as text inside a user turn.
//
// Only OpenAI-compatible backends (POST /v1/completions) implement it, and not
// every gateway in front of one serves the endpoint, so callers treat an error
// as "use chat instead" rather than as a failure of the model.
type RawCompleter interface {
	CompleteRaw(ctx context.Context, prompt string) (*Response, error)
}

// AsRawCompleter returns the RawCompleter behind p, looking through the
// resilience and blocking wrappers NewProvider adds. The wrappers are bypassed
// on purpose: a raw completion is issued by inline completion, which turns
// retries off anyway - a retried suggestion lands after the user typed past it.
func AsRawCompleter(p Provider) (RawCompleter, bool) {
	for p != nil {
		if rc, ok := p.(RawCompleter); ok {
			return rc, true
		}
		u, ok := p.(interface{ Unwrap() Provider })
		if !ok {
			return nil, false
		}
		p = u.Unwrap()
	}
	return nil, false
}
