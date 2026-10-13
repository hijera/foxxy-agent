Feature: Recover the answer card of a waiting question
  A missed stream event must not leave only an unclickable tool record.

  Scenario: Recover the choices from current server activity
    Given a question is still waiting on the server but its stream event was missed
    Then the chat restores selectable answer options from session activity
    And the selected answer uses the original request id
