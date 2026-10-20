Feature: Context compaction engine parity
  FoxxyCode ships two compaction engines. The coddy engine inserts a summary row and replays only
  the window after it; the opencode engine flags the folded messages and filters them from the
  payload. Whichever engine is active, the context usage published over ACP must describe the
  window that is actually sent to the model, so the composer ring drops right after compaction.

  Scenario Outline: Either engine publishes the compacted window over ACP
    Given the "<engine>" compaction engine is active
    And a session with 4 completed exchanges
    And the ACP client has observed the context usage before compaction
    When the session is compacted by the active engine
    Then the ACP client receives a smaller context usage update
    And the reported ACP usage matches the compacted LLM context
    And the LLM context window no longer holds the older exchanges

    Examples:
      | engine   |
      | coddy    |
      | opencode |

  Scenario Outline: Either engine keeps every original exchange in the transcript
    Given the "<engine>" compaction engine is active
    And a session with 4 completed exchanges
    When the session is compacted by the active engine
    Then the compaction summary is inserted into the transcript
    And the transcript still contains all 4 original exchanges

    Examples:
      | engine   |
      | coddy    |
      | opencode |

  Scenario Outline: The model compacts the session itself on either engine
    Given the "<engine>" compaction engine is active
    And a session with 4 completed exchanges
    When the model calls the compact_context tool
    Then the compaction summary is inserted into the transcript
    And the tool result says what was compacted
    And the LLM context window no longer holds the older exchanges

    Examples:
      | engine   |
      | coddy    |
      | opencode |

  Scenario Outline: A history the summarizer cannot read in one request is folded in passes on either engine
    Given the "<engine>" compaction engine is active
    And a session with 4 completed exchanges
    And the summarizer model has room for only part of the history per request
    When the session is compacted by the active engine
    Then the summarizer was called more than once
    And every summarization request after the first carries the summary so far
    And the compaction summary is inserted into the transcript

    Examples:
      | engine   |
      | coddy    |
      | opencode |
