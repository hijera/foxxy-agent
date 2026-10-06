package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// visionRejectingProvider refuses any request whose messages carry images, the way
// an OpenAI-compatible gateway in front of a text-only model does, and records the
// messages of every attempt.
type visionRejectingProvider struct {
	rejection string
	attempts  [][]Message
}

func (p *visionRejectingProvider) record(messages []Message) error {
	p.attempts = append(p.attempts, append([]Message(nil), messages...))
	for _, m := range messages {
		if len(m.ImageParts) > 0 {
			return errors.New(p.rejection)
		}
	}
	return nil
}

func (p *visionRejectingProvider) Complete(_ context.Context, messages []Message, _ []ToolDefinition) (*Response, error) {
	if err := p.record(messages); err != nil {
		return nil, err
	}
	return &Response{Content: "ok", StopReason: "end_turn"}, nil
}

func (p *visionRejectingProvider) Stream(_ context.Context, messages []Message, _ []ToolDefinition, onChunk func(StreamChunk)) (*Response, error) {
	if err := p.record(messages); err != nil {
		return nil, err
	}
	if onChunk != nil {
		onChunk(StreamChunk{TextDelta: "ok"})
	}
	return &Response{Content: "ok", StopReason: "end_turn"}, nil
}

func visionMessages() []Message {
	return []Message{
		{Role: RoleUser, Content: "look at this"},
		{Role: RoleUser, Content: "screenshot follows", ImageParts: []ImagePart{
			{DataURL: "data:image/png;base64,iVBORw0KGgo=", Name: "shot.png"},
		}},
	}
}

// The rejection wordings actually observed in the wild. api.neuraldeep.ru (LiteLLM)
// answers the first two for a text-only model group such as gpt-oss-20b.
var visionRejections = []string{
	`openai stream: POST "https://api.neuraldeep.ru/v1/chat/completions": 405 {"error":{"message":"Model openai/gpt-oss-20b does not accept image input. Received Model Group=gpt-oss-20b"}}`,
	`litellm.NotFoundError: NotFoundError: {"error":{"message":"No endpoints found that support image input","code":404}}`,
	`400 Invalid content type. image_url is only supported by certain models.`,
	`this model does not support image input`,
}

// TestResilientRetriesWithoutImagesOnVisionRejection pins the core behavior: a
// provider that rejects image input must not kill the turn. The request is
// re-issued once with the images stripped, so a browser screenshot degrades to a
// text note instead of an error the user sees.
func TestResilientRetriesWithoutImagesOnVisionRejection(t *testing.T) {
	for i, rejection := range visionRejections {
		t.Run(fmt.Sprintf("rejection_%d", i), func(t *testing.T) {
			inner := &visionRejectingProvider{rejection: rejection}
			p := wrapResilient(inner, ResilientOptions{RetryDisabled: true, Logger: discardLogger()})

			resp, err := p.Complete(context.Background(), visionMessages(), nil)
			if err != nil {
				t.Fatalf("Complete: %v", err)
			}
			if resp.Content != "ok" {
				t.Errorf("content = %q, want ok", resp.Content)
			}
			if len(inner.attempts) != 2 {
				t.Fatalf("attempts = %d, want 2 (with images, then without)", len(inner.attempts))
			}
			for _, m := range inner.attempts[1] {
				if len(m.ImageParts) > 0 {
					t.Fatal("second attempt still carried images")
				}
			}
		})
	}
}

// TestVisionFallbackTellsTheModelImagesWereDropped keeps the model from reasoning
// about a screenshot it was never shown.
func TestVisionFallbackTellsTheModelImagesWereDropped(t *testing.T) {
	inner := &visionRejectingProvider{rejection: visionRejections[0]}
	p := wrapResilient(inner, ResilientOptions{RetryDisabled: true, Logger: discardLogger()})

	if _, err := p.Complete(context.Background(), visionMessages(), nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	var joined strings.Builder
	for _, m := range inner.attempts[1] {
		joined.WriteString(m.Content)
		joined.WriteString("\n")
	}
	if !strings.Contains(joined.String(), imagesDroppedMarker) {
		t.Errorf("stripped messages do not mention the dropped image: %q", joined.String())
	}
	// The user's own words must survive the rewrite.
	if !strings.Contains(joined.String(), "screenshot follows") {
		t.Errorf("original text was lost: %q", joined.String())
	}
}

// TestVisionFallbackAppliesToStream covers the streaming transport, which is the
// one the agent actually uses.
func TestVisionFallbackAppliesToStream(t *testing.T) {
	inner := &visionRejectingProvider{rejection: visionRejections[0]}
	p := wrapResilient(inner, ResilientOptions{RetryDisabled: true, Logger: discardLogger()})

	var got strings.Builder
	resp, err := p.Stream(context.Background(), visionMessages(), nil, func(c StreamChunk) {
		got.WriteString(c.TextDelta)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if resp.Content != "ok" || got.String() != "ok" {
		t.Errorf("resp=%q chunks=%q, want ok", resp.Content, got.String())
	}
	if len(inner.attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(inner.attempts))
	}
}

// TestVisionFallbackIgnoresUnrelatedErrors keeps the fallback narrow: a failure
// that has nothing to do with images must surface unchanged, not be retried with
// a silently degraded request.
func TestVisionFallbackIgnoresUnrelatedErrors(t *testing.T) {
	inner := &stubProvider{completeFn: func(_ context.Context, _ []Message, _ []ToolDefinition) (*Response, error) {
		return nil, errors.New("401 invalid api key")
	}}
	p := wrapResilient(inner, ResilientOptions{RetryDisabled: true, Logger: discardLogger()})

	if _, err := p.Complete(context.Background(), visionMessages(), nil); err == nil {
		t.Fatal("expected the original error to surface")
	} else if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error = %v, want the original", err)
	}
}

// TestVisionFallbackSkippedWhenNoImages avoids a pointless second call when there
// was nothing to strip in the first place.
func TestVisionFallbackSkippedWhenNoImages(t *testing.T) {
	var calls int
	inner := &stubProvider{completeFn: func(_ context.Context, _ []Message, _ []ToolDefinition) (*Response, error) {
		calls++
		return nil, errors.New("model does not accept image input")
	}}
	p := wrapResilient(inner, ResilientOptions{RetryDisabled: true, Logger: discardLogger()})

	if _, err := p.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (nothing to strip, so no second attempt)", calls)
	}
}
