Feature: Reopen the last session of a project
  Editor plugins (VS Code, IntelliJ) open their panel on the bare SPA route and
  bind the backend to a fresh port on every IDE launch. FoxxyCode records the
  session the user last had open per project so the panel can continue where
  they left off instead of showing a new chat.

  Background:
    Given a running foxxycode HTTP server for project "app"

  Scenario: No session recorded yet
    When I ask which session to reopen
    Then no session is offered for reopening

  Scenario: The recorded session is offered on the next launch
    Given a session in project "app"
    And the plugin recorded that session as last opened
    When the plugin restarts on a new port
    And I ask which session to reopen
    Then the recorded session is offered for reopening

  Scenario: Starting a new chat clears the record
    Given a session in project "app"
    And the plugin recorded that session as last opened
    When the plugin records an empty session
    And I ask which session to reopen
    Then no session is offered for reopening

  Scenario: A deleted session is never offered
    Given a session in project "app"
    And the plugin recorded that session as last opened
    When that session is deleted
    And I ask which session to reopen
    Then no session is offered for reopening

  Scenario: A session from another project is never offered
    Given a session in folder "elsewhere" outside the project
    And the plugin recorded that session as last opened
    When I ask which session to reopen
    Then no session is offered for reopening
