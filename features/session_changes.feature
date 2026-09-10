Feature: Reviewing what a session changed
  Every turn is captured as a workspace snapshot diff, so FoxxyCode can tell the
  user which files a session touched without asking the agent to report it. The
  changed-files card under the transcript reads that change set, its viewer
  reads the per-file diff, and one button rolls the whole session back.

  Background:
    Given a running foxxycode HTTP server with a workspace
    And the workspace contains "notes.txt" with "one\ntwo\nthree\n"

  Scenario: A session with no edits has nothing to show
    When I ask what the session changed
    Then no files are reported as changed

  Scenario: An edited file is reported with its line counts
    When the agent runs a turn that writes "notes.txt" as "one\nTWO\nthree\n"
    And I ask what the session changed
    Then 1 file is reported as changed with 1 addition and 1 deletion
    And "notes.txt" is reported as "modified"

  Scenario: A created file is reported as an addition
    When the agent runs a turn that writes "extra.txt" as "fresh\n"
    And I ask what the session changed
    Then "extra.txt" is reported as "added"

  Scenario: Edits across several turns collapse into one net change
    When the agent runs a turn that writes "notes.txt" as "one\nTWO\nthree\n"
    And the agent runs a turn that writes "notes.txt" as "one\nTWO\nTHREE\n"
    And I ask what the session changed
    Then 1 file is reported as changed with 2 additions and 2 deletions

  Scenario: The review window can narrow the set to the last turn
    When the agent runs a turn that writes "notes.txt" as "one\nTWO\nthree\n"
    And the agent runs a turn that writes "extra.txt" as "fresh\n"
    And I ask what the last turn changed
    Then 1 file is reported as changed with 1 addition and 0 deletions
    And "extra.txt" is reported as "added"

  Scenario: The viewer reads the diff of one file
    When the agent runs a turn that writes "notes.txt" as "one\nTWO\nthree\n"
    And I open the diff for "notes.txt"
    Then the diff removes "two" and adds "TWO"

  Scenario: Rolling the session back restores the workspace
    When the agent runs a turn that writes "notes.txt" as "one\nTWO\nthree\n"
    And the agent runs a turn that writes "extra.txt" as "fresh\n"
    And I roll the session changes back
    Then "notes.txt" contains "one\ntwo\nthree\n"
    And "extra.txt" no longer exists
    And no files are reported as changed
