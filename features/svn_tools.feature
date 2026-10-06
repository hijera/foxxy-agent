Feature: Subversion tools
  With a working copy in the session folder the agent drives Subversion through
  dedicated svn_* tools instead of raw shell commands: inspection tools run
  freely, everything that changes the working copy or the repository asks for
  permission first, and the whole set disappears when Subversion support is
  turned off in the settings.

  Background:
    Given a fake svn client
    And an svn working copy on branch "trunk"

  Scenario: Inspect the working copy
    When the model calls svn_info
    Then the tool result reports branch "trunk"
    And the tool result reports revision 12

  Scenario: Review local changes
    When the model calls svn_status
    Then the tool result contains "M       src/main.go"
    When the model calls svn_diff
    Then the tool result contains "+// changed"

  Scenario: Commit reviewed paths
    When the model commits "src/main.go" with the message "fix: relay svn output"
    Then the svn client was called with "commit --message fix: relay svn output -- src/main.go"
    And the tool result contains "Committed revision 13"

  Scenario: Update the working copy before working on it
    When the model calls svn_update
    Then the tool result contains "At revision 13"

  Scenario: Switch the working copy to another branch
    When the model switches to branch "branches/feature-x"
    Then the svn client was called with "switch -- https://svn.example.test/repo/branches/feature-x"
    And a following svn_info reports branch "branches/feature-x"

  Scenario: Merge another branch into the working copy
    When the model merges from branch "branches/release-1"
    Then the svn client was called with "merge -- https://svn.example.test/repo/branches/release-1"
    And the tool result contains "Merging"

  Scenario: A merge conflict is reported back to the model
    Given the svn client reports a conflict on merge
    When the model merges from branch "branches/release-1"
    Then the tool result contains "E155015"

  Scenario: Check a branch out into its own folder
    When the model checks out branch "branches/feature-x" into "feature-folder"
    Then the tool result contains "Checked out revision"
    And the folder "feature-folder" is an svn working copy

  Scenario: Mutating tools ask for permission, inspection tools do not
    Then the tools "svn_info, svn_status, svn_diff, svn_log, svn_list" run without permission
    And the tools "svn_add, svn_revert, svn_resolve, svn_update, svn_commit, svn_switch, svn_merge, svn_checkout" require permission

  Scenario: A git repository inside the branch folder does not disturb the tools
    Given the working copy also holds a git repository
    When the model calls svn_info
    Then the tool result reports branch "trunk"
    When the model commits "src/main.go" with the message "fix: leave git alone"
    Then the svn client was called with "commit --message fix: leave git alone -- src/main.go"

  Scenario: Turning Subversion off removes the tools from the model
    Given subversion support is disabled in the settings
    Then no svn tool is offered to the model

  Scenario: Turning Subversion back on restores the tools without a restart
    Given subversion support is disabled in the settings
    And subversion support is enabled again in the settings
    Then the tools "svn_info, svn_status, svn_commit" are offered to the model
