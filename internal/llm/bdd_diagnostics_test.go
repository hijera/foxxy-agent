package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestLLMDiagnosticsFeature(t *testing.T) {
	var output bytes.Buffer
	var ctx context.Context
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^LLM debug logging is enabled$`, func() {
				output.Reset()
				ctx = WithDiagnostics(context.Background(), slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
			})
			sc.Step(`^a model streams a response over HTTP$`, func() error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"secret-answer\"}}]}\n\ndata: [DONE]\n\n")
				}))
				defer srv.Close()
				p, err := NewProvider(ProviderInput{Type: "openai", APIKey: "secret-key", BaseURL: srv.URL, Model: "test"})
				if err != nil {
					return err
				}
				_, err = p.Stream(ctx, []Message{{Role: RoleUser, Content: "secret-prompt"}}, nil, func(StreamChunk) {})
				return err
			})
			sc.Step(`^the log identifies network stages and the first model output without message contents$`, func() error {
				for _, want := range []string{"llm.call.start", "llm.http.start", "llm.http.connect_start", "llm.http.connection", "llm.http.request_written", "llm.http.first_response_byte", "llm.http.headers", "llm.http.first_body_byte", "llm.first_chunk", "llm.call.end", "elapsed_ms=", "call_id="} {
					if !strings.Contains(output.String(), want) {
						return fmt.Errorf("missing %s in %s", want, output.String())
					}
				}
				for _, secret := range []string{"secret-answer", "secret-key", "secret-prompt"} {
					if strings.Contains(output.String(), secret) {
						return fmt.Errorf("logged %s", secret)
					}
				}
				return nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/llm_diagnostics.feature"}, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("LLM diagnostics feature failed")
	}
}
