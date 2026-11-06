Feature: A subagent rides out a dropped provider connection
  A subagent runs unattended: nobody reads its transcript while it works, and
  nobody can type "continue" into it when its model provider drops the
  connection (issue #389). A dropped connection is a failure of the lane, not of
  the task, so the child keeps the part of the answer it had written, waits for
  the provider and goes on in the same session, and its task log says it is
  reconnecting instead of going quiet. A connection the remote host forcibly
  closed on Windows (WSAECONNRESET) is such a failure just like a reset on Linux.

  When the provider stays down longer than the child waits, the run fails and
  its transcript keeps everything it did. The parent continues that same child
  with spawn_agent resume instead of starting a second subagent on the same task
  from an empty context.

  Scenario: A subagent whose connection is forcibly closed mid-answer reconnects and finishes
    Given a workspace with a subagent definition "reviewer" under .foxxycode/agents
    And a parent agent session in that workspace
    And the workspace definition "reviewer" is approved for that workspace
    And the remote host forcibly closes the child's provider connection after "Reading the files" on its first call
    When the parent model spawns "reviewer" in the foreground and the child answers "REPORT: two findings"
    Then the spawn_agent tool result contains "REPORT: two findings"
    And the spawn_agent tool result reports the status "succeeded"
    And the child's task log says the subagent is reconnecting to its provider
    And the child session transcript keeps "Reading the files" and ends with "REPORT: two findings"
    And the parent session ran 1 subagent run on 1 child session

  Scenario: A subagent waits out more failed calls than an interactive turn does
    Given a workspace with a subagent definition "reviewer" under .foxxycode/agents
    And a parent agent session in that workspace
    And the workspace definition "reviewer" is approved for that workspace
    And the child's provider connection is reset on its first 3 calls
    When the parent model spawns "reviewer" in the foreground and the child answers "REPORT: patient"
    Then the spawn_agent tool result contains "REPORT: patient"
    And the spawn_agent tool result reports the status "succeeded"
    And the child's task log counts 3 reconnects

  Scenario: The parent resumes a subagent that failed on a provider outage
    Given a workspace with a subagent definition "reviewer" under .foxxycode/agents
    And a parent agent session in that workspace
    And the workspace definition "reviewer" is approved for that workspace
    And the child's provider refuses every connection
    When the parent model spawns "reviewer" in the foreground and the child answers "REPORT: resumed"
    Then the spawn_agent tool result reports the status "failed"
    And the spawn_agent tool result tells the parent to resume that subagent
    When the child's provider accepts connections again
    And the parent model resumes that subagent with "Continue where you stopped"
    Then the spawn_agent tool result contains "REPORT: resumed"
    And the spawn_agent tool result reports the status "succeeded"
    And the child session transcript holds the message "Continue where you stopped" and ends with "REPORT: resumed"
    And the parent session ran 2 subagent runs on 1 child session
