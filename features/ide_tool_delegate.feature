Feature: IDE tool delegation
  A tool call can be offered to a connected IDE client - an editor plugin that
  declared the tool on its /foxxycode/ide/events subscription - which executes
  it through the editor (for example write/edit/apply_patch through the
  document model) and returns the tool result. A call the client declines runs
  in the agent as before.

  Scenario: The IDE client executes the tool call
    Given a foxxycode session in "agent" mode
    And an IDE client that handles the "write" tool
    And a model that requests the "write" tool once, then answers
    When the user asks for a change
    Then the IDE client received the "write" call
    And the file is not written
    And the tool result carries the IDE client's output
    And the turn ends with the model's answer

  Scenario: A call the IDE client declines runs locally
    Given a foxxycode session in "agent" mode
    And an IDE client that declines the "write" tool
    And a model that requests the "write" tool once, then answers
    When the user asks for a change
    Then the file is written
    And the turn ends with the model's answer
