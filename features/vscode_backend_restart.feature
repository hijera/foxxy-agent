Feature: VS Code panels follow the restarted backend
  Restart replaces the one backend shared by the sidebar and editor tab.

  Scenario: The outgoing process exits after its replacement starts
    Then the replacement remains running when the previous backend exits

  Scenario: Both panels are open during restart
    Then the editor tab and sidebar reconnect to the replacement URL

  Scenario: The editor tab closes while the replacement starts
    Then the remaining sidebar reconnects and the editor tab stays closed
