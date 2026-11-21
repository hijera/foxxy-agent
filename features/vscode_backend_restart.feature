Feature: VS Code panels follow the restarted backend
  Restart replaces the one backend shared by the sidebar and editor tab.

  Scenario: The outgoing process exits after its replacement starts
    When the previous backend exits after its replacement has started
    Then the replacement remains running and keeps the new address

  Scenario: Both panels are open during restart
    When Restart replaces the backend with the editor tab and the sidebar open
    Then both panels reconnect to the replacement address

  Scenario: The editor tab closes while the replacement starts
    When the editor tab closes while the replacement backend is starting
    Then the remaining sidebar reconnects and the editor tab stays closed
