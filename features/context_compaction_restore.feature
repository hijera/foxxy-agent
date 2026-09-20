Feature: Context usage survives a reload
  The context estimate is persisted next to the provider token counters, so a session reopened
  after a restart reports the compacted window instead of its pre-compaction size. This holds for
  both compaction engines, and both answer the manual /compact command.

  Scenario Outline: A reopened session reports the compacted context on either engine
    Given an HTTP session with 4 completed exchanges on the "<engine>" compaction engine
    When the user sends a regular prompt
    Then the agent reply arrives over HTTP
    And the session transcript contains a compaction summary row
    And HTTP session stats match the compacted LLM context
    When the server is restarted against the same session store
    Then HTTP session stats match the compacted LLM context

    Examples:
      | engine   |
      | coddy    |
      | opencode |

  Scenario Outline: Either engine advertises the manual compact command
    Given an HTTP session with 4 completed exchanges on the "<engine>" compaction engine
    Then the slash command catalog offers "/compact"

    Examples:
      | engine   |
      | coddy    |
      | opencode |

  Scenario Outline: The REST compaction folds the history with the engine the session runs on
    Given an HTTP session with 4 completed exchanges on the "<engine>" compaction engine
    When the client posts to the session compact endpoint
    Then the compact request succeeds
    And the session transcript contains a compaction summary row
    And HTTP session stats match the compacted LLM context

    Examples:
      | engine   |
      | coddy    |
      | opencode |
