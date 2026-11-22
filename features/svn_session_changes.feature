Feature: Reviewing and discarding Subversion edits in a session
  Edits shows file changes against the local checkout's base revision
  and can hand the same text snapshots to the native editor.

  Scenario: Review an SVN change made by the agent and discard working copy edits
    Given a running foxxycode HTTP server whose workspace is an SVN working copy
    And SVN has "notes @ draft.txt" committed as "one\ntwo\nthree\n"
    When the agent runs a turn that writes "notes @ draft.txt" as "one\nTWO\nthree\n"
    And the agent runs a turn that writes "extra.txt" as "fresh\n"
    And I ask what the working copy changed
    Then svn is named as the version control
    And 2 files are reported as changed with 2 additions and 1 deletion
    And "notes @ draft.txt" is reported as "modified"
    And "extra.txt" is reported as "added"
    When I open the diff for "notes @ draft.txt"
    Then the diff removes "two" and adds "TWO"
    And the local editor receives the reviewed snapshots
    When I discard the reviewed revision of "notes @ draft.txt"
    Then "notes @ draft.txt" contains "one\ntwo\nthree\n"
    And "extra.txt" contains "fresh\n"
    When I discard every change
    Then "extra.txt" no longer exists
    When I ask what the working copy changed
    Then no files are reported as changed
