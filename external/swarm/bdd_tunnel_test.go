//go:build swarm

package swarm

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cucumber/godog"
)

func TestSwarmTunnelFeature(t *testing.T) {
	var stand *tunnelStand
	var status int
	var body string
	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			ctx.Step(`^a fresh relay and a node connected only by an outbound tunnel$`, func() {
				stand = newTunnelStand(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = fmt.Fprint(w, r.URL.Path)
				}))
			})
			ctx.Step(`^a client requests the node sessions through the relay$`, func() {
				status, body = stand.get(t, "/foxxycode/sessions")
			})
			ctx.Step(`^the tunneled node receives the sessions request$`, func() error {
				if status != http.StatusOK || body != "/foxxycode/sessions" {
					return fmt.Errorf("tunnel response: status %d, body %q", status, body)
				}
				return nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/swarm_tunnel.feature"}, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("swarm tunnel feature failed")
	}
}
