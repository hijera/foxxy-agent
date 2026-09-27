Feature: A session shared by two processes over one home
  An editor panel's `foxxycode http` and the Telegram gateway each keep a
  session in memory once both hold it: the panel shows it, the chat /resume'd
  it. The turn lock keeps their turns apart, and a turn re-reads what the other
  process wrote before it starts, so neither answers on a stale history nor
  writes that history back over the other one's turn.

  Background:
    Given a session held live by the panel and the gateway
    And the panel ran a turn "one"

  Scenario: A turn begins on the history the other process wrote
    When the panel runs a turn "two"
    And the gateway runs a turn "three"
    Then the gateway's turn began on 4 messages
    And the transcript on disk is "one | panel: one | two | panel: two | three | gateway: three"

  Scenario: A settings change in the process that fell behind leaves the newer transcript alone
    When the panel runs a turn "two"
    And the gateway switches the session to "plan" mode
    Then the transcript on disk is "one | panel: one | two | panel: two"
    And the session on disk is in "plan" mode
