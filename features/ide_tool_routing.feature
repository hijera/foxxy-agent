Feature: IDE tool execution has one owner
  Scenario: Two capable editors and an observer share the event stream
    Given two IDE clients that execute commands and one observer
    When the HTTP bridge delegates a command
    Then exactly one capable client receives the command
    And the observer receives no execution request
    And the bridge returns the selected client's result
