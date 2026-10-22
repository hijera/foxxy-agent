Feature: Queued prompts run after the model reaches its output limit
  Scenario: The output cap delivers unread steering before deferred prompts
    Given a model that reaches its 4096 token output limit with both queue modes waiting
    When the user starts that output-limited turn
    Then the unread steering runs before the two deferred prompts
    And each queued message appears exactly once in the transcript
    And the truncated answer and its limit notice remain in the transcript
    And no queued messages remain after the deferred answers
