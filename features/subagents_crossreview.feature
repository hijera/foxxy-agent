Feature: A coordinator subagent delegates a bounded fan-out
  An orchestrator such as crossreview needs one level of delegation past the
  depth cap: the coordinator spawns reviewers itself. A definition names the
  children it may hand work to with `spawns`; the runtime admits exactly those
  names exactly one level past subagents.max_depth, and the deeper children
  never spawn again.

  Scenario: A coordinator delegates one level past the depth cap
    Given the user scope holds a subagent definition "reviewer"
    And the user scope holds a subagent definition "coordinator" that may spawn "reviewer"
    And a parent agent session in that workspace
    When the parent model spawns "coordinator" in the foreground and it delegates to "reviewer" before answering "REPORT: quorum merged"
    Then the spawn_agent tool result contains "REPORT: quorum merged"
    And a child session ran as subagent "reviewer"
    And the "reviewer" child was not offered the spawn_agent tool

  Scenario: The allowlist admits only the names on it
    Given the user scope holds a subagent definition "reviewer"
    And the user scope holds a subagent definition "other"
    And the user scope holds a subagent definition "coordinator" that may spawn "reviewer"
    And a parent agent session in that workspace
    When the parent model spawns "coordinator" in the foreground and it delegates to "other" before answering "REPORT: refused"
    Then the delegation to "other" inside "coordinator" was refused naming the spawns allowlist
