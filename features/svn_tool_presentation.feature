Feature: SVN tool presentation
  Scenario: Review Subversion operations in the transcript
    Given a transcript containing SVN tool calls
    When I expand the SVN calls
    Then SVN operations have readable headings and paths
    And SVN status distinguishes property and tree conflicts
    And SVN diffs retain their content with highlighted changes
    And SVN errors and commit approvals are clearly presented
