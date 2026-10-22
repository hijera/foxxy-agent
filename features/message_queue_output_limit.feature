Feature: Deferred prompts run after the model reaches its output limit
  Scenario: The output cap starts deferred prompts without replaying unread steering
    Given a model that reaches its 4096 token output limit with both queue modes waiting
    When the user starts that output-limited turn
    Then the two deferred prompts run once each in order
    And the unread steering message is not sent to the model
    And the truncated answer and its limit notice remain in the transcript
    And no queued messages remain after the deferred answers
