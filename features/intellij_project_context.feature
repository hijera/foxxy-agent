Feature: IntelliJ IDEA metadata is attached to the session context
  IntelliJ projects describe their modules and required plugins in files under
  .idea. The model needs that metadata from the first request, without waiting
  for a user to mention or manually attach those files.

  Scenario: The first model request carries IntelliJ module and plugin metadata
    Given a project with IntelliJ module and plugin metadata
    And a foxxycode agent session in that IntelliJ project
    When the user asks about the project setup
    Then the first model request contains the IntelliJ module and plugin metadata
