Feature: Context usage survives a reload
  The context estimate is persisted next to the provider token counters, so a session reopened
  after a restart reports the compacted window instead of its pre-compaction size. The same
  engine answers the manual /compact command.

  Scenario: A reopened session reports the compacted context
    Given an HTTP session with 4 completed exchanges and a small context window
    When the user sends a regular prompt
    Then the agent reply arrives over HTTP
    And the session transcript contains a compaction summary row
    And HTTP session stats match the compacted LLM context
    When the server is restarted against the same session store
    Then HTTP session stats match the compacted LLM context

  Scenario: The server advertises the manual compact command
    Given an HTTP session with 4 completed exchanges and a small context window
    Then the slash command catalog offers "/compact"

  Scenario: The REST compaction folds the history through Coddy
    Given an HTTP session with 4 completed exchanges and a small context window
    When the client posts to the session compact endpoint
    Then the compact request succeeds
    And the session transcript contains a compaction summary row
    And HTTP session stats match the compacted LLM context
