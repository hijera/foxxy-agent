Feature: Saving diagrams in the VS Code panel
  The editor saves a diagram through its host file API, since a blob download
  inside the webview can be cancelled by Electron.

  Scenario: Save a diagram from the embedded chat
    When a reader downloads a diagram in the VS Code panel
    Then the source reaches the editor host unchanged
