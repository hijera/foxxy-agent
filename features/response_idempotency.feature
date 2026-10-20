Feature: A retried response request does not run the agent again
  Scenario: Replay the same request after a server restart
    Given a persisted response session
    When a response request with an idempotency key completes
    And the server is restarted and the same request is sent again
    Then the original response is replayed without another agent run
