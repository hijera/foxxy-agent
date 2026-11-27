Feature: A live session owns its Chrome browser
  Scenario: Reopening the chat reuses Chrome and releasing the session closes it
    Given a live session with a running browser
    When a new browser tool registry opens that session's browser
    Then it reuses the original Chrome connection
    And releasing the session closes Chrome and prevents a late relaunch
