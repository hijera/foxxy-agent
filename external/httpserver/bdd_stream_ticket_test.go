//go:build http

package httpserver

// Godog harness for features/stream_ticket.feature: drives the mint endpoint and
// the SSE auth path over a real httptest server.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/config"
)

type streamTicketFeatureState struct {
	t          *testing.T
	ts         string
	authToken  string
	ticket     string
	mintStatus int
	lastStatus int
}

func (s *streamTicketFeatureState) givenServer(token string) error {
	_, ts := authTestServer(s.t, cfgWithAuth(token))
	s.ts = ts.URL
	s.authToken = token
	s.ticket = ""
	s.mintStatus = 0
	s.lastStatus = 0
	return nil
}

func (s *streamTicketFeatureState) mint(withAuth bool) error {
	req, err := http.NewRequest(http.MethodPost, s.ts+"/foxxycode/stream-tickets", nil)
	if err != nil {
		return err
	}
	if withAuth {
		req.Header.Set("Authorization", "Bearer "+s.authToken)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	s.mintStatus = res.StatusCode
	if res.StatusCode != http.StatusOK {
		return nil
	}
	var parsed struct {
		Object    string `json:"object"`
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expiresIn"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return err
	}
	if parsed.Object != "foxxycode.stream_ticket" {
		return fmt.Errorf("object = %q, want foxxycode.stream_ticket", parsed.Object)
	}
	if parsed.ExpiresIn <= 0 {
		return fmt.Errorf("expiresIn = %d, want a positive TTL", parsed.ExpiresIn)
	}
	s.ticket = parsed.Ticket
	return nil
}

func (s *streamTicketFeatureState) mintWithAuth() error    { return s.mint(true) }
func (s *streamTicketFeatureState) mintWithoutAuth() error { return s.mint(false) }

func (s *streamTicketFeatureState) ticketIsNotTheAuthToken() error {
	if strings.TrimSpace(s.ticket) == "" {
		return fmt.Errorf("no ticket in the mint response")
	}
	if s.ticket == s.authToken {
		return fmt.Errorf("the mint handed back the durable auth token")
	}
	return nil
}

// subscribeWithTicket hits the events SSE route with the ticket in the query string.
// The handler streams, so the request is cancelled as soon as the status is known.
func (s *streamTicketFeatureState) subscribeWithTicket() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.ts+"/foxxycode/events?access_token="+s.ticket, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	s.lastStatus = res.StatusCode
	_ = res.Body.Close()
	return nil
}

func (s *streamTicketFeatureState) sessionListWithTicket() error {
	req, err := http.NewRequest(http.MethodGet, s.ts+"/foxxycode/sessions?access_token="+s.ticket, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	s.lastStatus = res.StatusCode
	return nil
}

func (s *streamTicketFeatureState) subscriptionAccepted() error {
	if s.lastStatus == http.StatusUnauthorized {
		return fmt.Errorf("subscription rejected with 401")
	}
	return nil
}

func (s *streamTicketFeatureState) lastRequestUnauthorized() error {
	if s.lastStatus != http.StatusUnauthorized {
		return fmt.Errorf("status = %d, want 401", s.lastStatus)
	}
	return nil
}

func (s *streamTicketFeatureState) mintUnauthorized() error {
	if s.mintStatus != http.StatusUnauthorized {
		return fmt.Errorf("mint status = %d, want 401", s.mintStatus)
	}
	return nil
}

func initializeStreamTicketScenario(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &streamTicketFeatureState{t: t}
		sc.Step(`^a running foxxycode server with the auth token "([^"]*)"$`, s.givenServer)
		sc.Step(`^I mint a stream ticket with the auth token$`, s.mintWithAuth)
		sc.Step(`^I mint a stream ticket without the auth token$`, s.mintWithoutAuth)
		sc.Step(`^the mint response carries a ticket that is not the auth token$`, s.ticketIsNotTheAuthToken)
		sc.Step(`^I subscribe to the events stream with the ticket$`, s.subscribeWithTicket)
		sc.Step(`^I subscribe to the events stream with the same ticket again$`, s.subscribeWithTicket)
		sc.Step(`^I request the session list with the ticket$`, s.sessionListWithTicket)
		sc.Step(`^the subscription is accepted$`, s.subscriptionAccepted)
		sc.Step(`^the subscription is rejected as unauthorized$`, s.lastRequestUnauthorized)
		sc.Step(`^the request is rejected as unauthorized$`, s.lastRequestUnauthorized)
		sc.Step(`^the mint request is rejected as unauthorized$`, s.mintUnauthorized)
	}
}

func TestStreamTicketFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "stream_ticket",
		ScenarioInitializer: initializeStreamTicketScenario(t),
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/stream_ticket.feature"},
			Output:   os.Stdout,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("stream_ticket feature failed")
	}
}

// cfgWithAuthTicketsOnly is cfgWithAuth plus httpserver.stream_tickets_only, the
// mode where a URL may never carry the durable credential.
func cfgWithAuthTicketsOnly(token string) *config.Config {
	c := cfgWithAuth(token)
	c.HTTPServer.StreamTicketsOnly = true
	return c
}
