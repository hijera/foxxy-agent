Feature: Subversion workspace support
  FoxxyCode detects Subversion working copies next to git ones, shows the SVN
  chip data in the workspace context, and switches svn branches either in place
  or into their own branch folder. Git and svn detection stay independent, so a
  branch folder checked out from SVN can also hold a git repository.

  Background:
    Given a running foxxycode HTTP server with a fake svn client

  Scenario: Workspace context inside an svn working copy
    Given an svn working copy "wc" on branch "trunk"
    And a session rooted at folder "wc"
    When I request the workspace context
    Then the context reports an svn working copy on branch "trunk"
    And the context reports svn revision 12
    And the context reports the svn url "https://svn.example.test/repo/trunk"

  Scenario: A plain folder is not an svn working copy
    Given a plain folder "plain"
    And a session rooted at folder "plain"
    When I request the workspace context
    Then the context reports it is not an svn working copy
    And the context reports the svn client is available

  Scenario: A git repository inside an svn branch folder reports both
    Given an svn working copy "wc" on branch "branches/feature-x"
    And folder "wc" is also a git repository on branch "main"
    And a session rooted at folder "wc"
    When I request the workspace context
    Then the context reports a git repository on branch "main"
    And the context reports an svn working copy on branch "branches/feature-x"

  Scenario: A git clone in an unversioned subfolder still sees the svn branch
    Given an svn working copy "wc" on branch "branches/feature-x"
    And an unversioned subfolder "vendor" inside "wc" that is a git repository on branch "main"
    And a session rooted at the subfolder "vendor"
    When I request the workspace context
    Then the context reports a git repository on branch "main"
    And the context reports an svn working copy on branch "branches/feature-x"
    And the context reports the svn working copy root is above the session folder

  Scenario: Listing branches for the svn chip menu
    Given an svn working copy "wc" on branch "trunk"
    And a session rooted at folder "wc"
    When I request the workspace context
    Then the context lists svn branches "trunk, branches/feature-x, branches/release-1"

  Scenario: Switch the svn working copy to another branch in place
    Given an svn working copy "wc" on branch "trunk"
    And a session rooted at folder "wc"
    When I switch the session to svn branch "branches/feature-x"
    Then the context reports an svn working copy on branch "branches/feature-x"
    And the session folder is unchanged

  Scenario: Check an svn branch out into its own folder
    Given an svn working copy "wc" on branch "trunk"
    And a session rooted at folder "wc"
    When I switch the session to svn branch "branches/feature-x" in a separate folder
    Then the context reports an svn working copy on branch "branches/feature-x"
    And the session moved to a new branch folder

  Scenario: Switching the git branch leaves the svn branch untouched
    Given an svn working copy "wc" on branch "trunk"
    And folder "wc" is also a git repository on branch "main"
    And the git repository has branch "feature/login"
    And a session rooted at folder "wc"
    When I switch the session to branch "feature/login"
    Then the context reports a git repository on branch "feature/login"
    And the context reports an svn working copy on branch "trunk"

  Scenario: Switching the svn branch leaves the git branch untouched
    Given an svn working copy "wc" on branch "trunk"
    And folder "wc" is also a git repository on branch "main"
    And a session rooted at folder "wc"
    When I switch the session to svn branch "branches/feature-x"
    Then the context reports an svn working copy on branch "branches/feature-x"
    And the context reports a git repository on branch "main"

  Scenario: Subversion support turned off in the settings
    Given an svn working copy "wc" on branch "trunk"
    And subversion support is disabled in the settings
    And a session rooted at folder "wc"
    When I request the workspace context
    Then the context reports it is not an svn working copy
    When I switch the session to svn branch "branches/feature-x"
    Then the workspace request fails with status 409

  Scenario: No svn client installed
    Given an svn working copy "wc" on branch "trunk"
    And the svn client is not installed
    And a session rooted at folder "wc"
    When I request the workspace context
    Then the context reports it is not an svn working copy
    And the context reports the svn client is unavailable
    When I switch the session to svn branch "branches/feature-x"
    Then the workspace request fails with status 409

  Scenario: The svn workspace is locked once the conversation starts
    Given an svn working copy "wc" on branch "trunk"
    And a session rooted at folder "wc"
    And the session already has a user message
    When I switch the session to svn branch "branches/feature-x"
    Then the workspace request fails with status 409
    And the context reports an svn working copy on branch "trunk"
