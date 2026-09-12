Feature: Todo tool cards keep their plan snapshot
  The chat UI renders foxxycode_todo_item_update and foxxycode_todo_plan_replace
  as a plan card only while the tool call carries the plan snapshot it produced.
  The snapshot lives next to the call's meta.json and must survive the call being
  marked finished, whichever write lands first: a transcript reload that reads the
  call in between otherwise degrades the card to a bare tool row.

  Background:
    Given a running foxxycode HTTP server
    And a session with a todo tool call "todo-update-1" that updated a two-item plan

  Scenario: Snapshot written before the call is marked finished
    When the plan snapshot is saved and then the call is marked completed
    And the UI lists the session's tool calls
    Then the tool call "todo-update-1" is completed and carries the two-item plan snapshot

  Scenario: Snapshot written after the call is marked finished
    When the call is marked completed and then the plan snapshot is saved
    And the UI lists the session's tool calls
    Then the tool call "todo-update-1" is completed and carries the two-item plan snapshot

  Scenario: A hub-style tool call id with a colon still keeps its snapshot
    Given a session with a todo tool call "functions.foxxycode_todo_item_update:3" that updated a two-item plan
    When the plan snapshot is saved and then the call is marked completed
    And the UI lists the session's tool calls
    Then the tool call "functions.foxxycode_todo_item_update:3" is completed and carries the two-item plan snapshot
