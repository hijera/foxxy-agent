Feature: Stop withdraws a pending tool permission
  A tool that never received permission must remain cancelled after the chat reloads.

  Scenario: Stop while a command awaits approval
    Given an agent whose command requires approval
    When the operator stops the turn at the permission prompt
    Then the command has a persisted cancelled result
    And the command was never executed
