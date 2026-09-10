Feature: Short-lived stream tickets for SSE subscriptions
  EventSource cannot set an Authorization header, so the SSE routes accept the
  credential in the query string — where it lands in access logs, proxy logs and
  browser history. An authenticated client can instead mint a single-use,
  short-lived ticket that only the SSE routes accept, so the value that leaks
  into a log is already spent by the time anyone reads it.

  Background:
    Given a running foxxycode server with the auth token "primary-secret"

  Scenario: A ticket authenticates one SSE subscription and is then spent
    When I mint a stream ticket with the auth token
    Then the mint response carries a ticket that is not the auth token
    When I subscribe to the events stream with the ticket
    Then the subscription is accepted
    When I subscribe to the events stream with the same ticket again
    Then the subscription is rejected as unauthorized

  Scenario: A ticket is refused on the regular API
    When I mint a stream ticket with the auth token
    And I request the session list with the ticket
    Then the request is rejected as unauthorized

  Scenario: Minting requires the real credential
    When I mint a stream ticket without the auth token
    Then the mint request is rejected as unauthorized
