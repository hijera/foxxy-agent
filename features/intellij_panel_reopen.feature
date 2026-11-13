Feature: Reopening the IntelliJ panel restores the chat
  A restarted backend must have a working embedded chat, not just a running process.

  Scenario: Reopen after stopping the backend on tab close
    Given the FoxxyCode panel is open in a real IntelliJ IDE
    And stopping the process on tab close is enabled
    When the panel is hidden
    Then the backend process exits
    When the panel is opened again
    Then a new backend process starts
    And the embedded chat renders its composer
