package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// errSentinel is wrapped by the stub below so a test can prove the label keeps
// errors.Is working for callers that classify by cause.
var errSentinel = errors.New("upstream said no")

type stubLabelProvider struct{ err error }

func (p *stubLabelProvider) Complete(context.Context, []Message, []ToolDefinition) (*Response, error) {
	return nil, p.err
}

func (p *stubLabelProvider) Stream(context.Context, []Message, []ToolDefinition, func(StreamChunk)) (*Response, error) {
	return nil, p.err
}

func TestLabelProviderNamesTheEntryAndTheAddress(t *testing.T) {
	p := labelProvider(&stubLabelProvider{err: errSentinel}, ProviderInput{
		Name: "rpa", Type: "openai", BaseURL: "https://api.rpa.icu/v1",
	})
	_, err := p.Complete(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("want the stub error back")
	}
	if !strings.Contains(err.Error(), `provider "rpa"`) {
		t.Errorf("error %q does not name the provider", err)
	}
	if !strings.Contains(err.Error(), "https://api.rpa.icu/v1") {
		t.Errorf("error %q does not name the address the request reached", err)
	}
	if !errors.Is(err, errSentinel) {
		t.Error("the label replaced the cause instead of wrapping it; errors.Is broke")
	}
}

// TestLabelProviderLeavesSuccessAlone keeps the wrapper out of the happy path:
// a call that worked must not grow an error.
func TestLabelProviderLeavesSuccessAlone(t *testing.T) {
	p := labelProvider(&stubLabelProvider{}, ProviderInput{Name: "rpa", Type: "openai"})
	if _, err := p.Stream(context.Background(), nil, nil, func(StreamChunk) {}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
}

// TestLabelProviderSkipsUnnamedInputs covers the helpers that build a provider ad
// hoc with no providers[] entry behind it: there is no name to report, so the
// wrapper must not add an empty one.
func TestLabelProviderSkipsUnnamedInputs(t *testing.T) {
	inner := &stubLabelProvider{err: errSentinel}
	if got := labelProvider(inner, ProviderInput{Type: "openai"}); got != Provider(inner) {
		t.Fatal("an unnamed input was wrapped")
	}
	if got := labelProvider(inner, ProviderInput{Name: "   ", Type: "openai"}); got != Provider(inner) {
		t.Fatal("a blank name was wrapped")
	}
}

func TestProviderEndpointReportsWhereRequestsGo(t *testing.T) {
	cases := []struct {
		name       string
		provType   string
		configured string
		want       string
	}{
		{"configured base wins", "openai", "https://proxy.example/v1", "https://proxy.example/v1"},
		{"openai default", "openai", "", openAIDefaultAPIBase},
		{"anthropic default", "anthropic", "", anthropicDefaultAPIBase},
		{"unknown type has no default", "somethingelse", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProviderEndpoint(tc.provType, tc.configured); got != tc.want {
				t.Errorf("ProviderEndpoint(%q, %q) = %q, want %q", tc.provType, tc.configured, got, tc.want)
			}
		})
	}
	// neuraldeep and codex pin their own address, environment overrides
	// included, so the report is never a guess: whatever those resolvers say is
	// what the provider itself will use.
	if got := ProviderEndpoint("neuraldeep", ""); got != neuralDeepAPIBase("") {
		t.Errorf("neuraldeep endpoint = %q, want the resolver's answer %q", got, neuralDeepAPIBase(""))
	}
	if got := ProviderEndpoint("codex", "https://ignored.example"); got != codexBaseURL() {
		t.Errorf("codex endpoint = %q, want the resolver's answer %q", got, codexBaseURL())
	}
}

// TestLabelSitsOutsideTheRetryClassifier is the reason the wrap order matters:
// a labelled error must still read as retryable, or the resilient layer would
// stop retrying the moment errors started carrying a provider name.
func TestLabelSitsOutsideTheRetryClassifier(t *testing.T) {
	inner := errors.New("openai stream: 503 Service Unavailable")
	if !IsRetryableProviderError(inner) {
		t.Fatal("the fixture is not retryable to begin with")
	}
	labelled := labelProvider(&stubLabelProvider{err: inner}, ProviderInput{Name: "rpa", Type: "openai"})
	_, err := labelled.Complete(context.Background(), nil, nil)
	if !IsRetryableProviderError(err) {
		t.Error("a labelled error stopped reading as retryable")
	}
	if got := HTTPStatus(err); got != 503 {
		t.Errorf("HTTPStatus of a labelled error = %d, want 503", got)
	}
}
