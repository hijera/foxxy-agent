Feature: Continue interrupted work without repeating the same actions
  Scenario: Recover after a process crash
    Given a persisted agent execution interrupted during a model call
    When the agent continues the saved conversation
    Then the model receives an interruption recovery instruction
    And the recovery instruction is not a user message in the transcript

  Scenario: Repeated unchanged tool results survive a restart
    Given an agent repeatedly reading unchanged information
    When the agent reaches its turn limit and starts again
    Then the model receives a loop correction before execution stops
