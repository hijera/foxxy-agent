Feature: Editor project metadata is attached to the session context
  IntelliJ projects describe their modules and required plugins under .idea, and
  VS Code workspaces describe their tasks, launch configurations and editor
  settings under .vscode. The model needs that metadata from the first request,
  without waiting for a user to mention or manually attach those files.

  Scenario: The first model request carries IntelliJ module and plugin metadata
    Given a project with IntelliJ module and plugin metadata
    And a foxxycode agent session in that IntelliJ project
    When the user asks about the project setup
    Then the first model request contains the IntelliJ module and plugin metadata

  Scenario: The first model request carries VS Code workspace settings
    Given a project with VS Code workspace settings and tasks
    And a foxxycode agent session in that VS Code project
    When the user asks about the project setup
    Then the first model request contains the VS Code workspace settings and tasks
