Feature: The first prompt after the backend starts
  Opening an editor panel fires every per-session route at once, and the first prompt follows
  them. Reading a chat may not turn into one full load per request, or the reads fight each
  other for the browser's handful of connections and the prompt never leaves it - which looks,
  from the chat, like a request that was sent and then ignored.

  Background:
    Given a running foxxycode HTTP server
    And a chat that was left on disk by an earlier run

  Scenario: A panel opens the chat and sends its first prompt
    When the panel opens every view of the chat at once
    Then every view answers
    And the chat was read from disk once
    When I send my first prompt
    Then the prompt reaches the model
