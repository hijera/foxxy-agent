Feature: tool call batches remain paired
  Scenario: cancellation closes a batch and follow-up sends valid history
    Given a batch of three shell tool calls
    When the second tool call cancels the turn
    Then only the first two tool calls execute
    And every call in the cancelled batch has exactly one result
    And the follow-up request has adjacent paired tool history

  Scenario: legacy missing output is repaired before resume
    Given a legacy history with one missing tool output
    When the agent resumes the session
    Then the legacy history remains unchanged on disk
    And the resume request has adjacent paired tool history
    And the resumed turn succeeds

  Scenario: stale permission is not executed after a newer user message
    Given a pending permission followed by a newer user message
    When the stale permission is allowed
    Then the stale permission gate is cleared
    And the newer user message remains in history
    And the provider receives no resume request
