//go:build http

package httpserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestIDEToolRoutingFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:    "ide-tool-routing",
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/ide_tool_routing.feature"}, TestingT: t},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			var clients []chan ideEvent
			var done chan ideExecOutcome
			var outcome ideExecOutcome
			var cancel context.CancelFunc
			var event ideEvent
			sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
				if cancel != nil {
					cancel()
				}
				for _, ch := range clients {
					ideEvents.Unsubscribe(ch)
				}
				return ctx, nil
			})
			sc.Step(`^two IDE clients that execute commands and one observer$`, func() {
				clients = []chan ideEvent{ideEvents.SubscribeHandled([]string{"run_command"}), ideEvents.SubscribeHandled([]string{"*"}), ideEvents.Subscribe()}
			})
			sc.Step(`^the HTTP bridge delegates a command$`, func() {
				ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
				cancel = stop
				done = make(chan ideExecOutcome, 1)
				s := NewSender(&config.Config{}, nil, false, "m")
				go func() {
					res, err := s.ExecuteToolCall(ctx, acp.ToolExecutionRequest{SessionID: "routing", ToolCallID: "routing-call", ToolName: "run_command", ArgsJSON: `{"command":"echo hello"}`})
					done <- ideExecOutcome{res, err}
				}()
			})
			sc.Step(`^exactly one capable client receives the command$`, func() error {
				select {
				case event = <-clients[0]:
				case event = <-clients[1]:
				case <-time.After(time.Second):
					return fmt.Errorf("no capable client received the call")
				}
				if !CompleteToolExecResult(event.ToolCallID, event.Nonce, acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: "selected client"}) {
					return fmt.Errorf("selected client's result was rejected")
				}
				// Await the bridge so dispatch has finished before inspecting queues.
				outcome = <-done
				for _, ch := range clients[:2] {
					select {
					case <-ch:
						return fmt.Errorf("a second client was asked to execute the same call")
					default:
					}
				}
				return nil
			})
			sc.Step(`^the observer receives no execution request$`, func() error {
				select {
				case <-clients[2]:
					return fmt.Errorf("observer received the execution nonce")
				default:
					return nil
				}
			})
			sc.Step(`^the bridge returns the selected client's result$`, func() error {
				if outcome.err != nil || outcome.res.Status != acp.ToolExecHandled || outcome.res.Output != "selected client" {
					return fmt.Errorf("unexpected result: %+v, %v", outcome.res, outcome.err)
				}
				return nil
			})
		},
	}
	if suite.Run() != 0 {
		t.Fatal("IDE routing feature failed")
	}
}
