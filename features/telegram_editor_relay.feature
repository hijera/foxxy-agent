Feature: A shared Telegram relay for editor projects
  Scenario: IntelliJ and VS Code share a bot without sharing incoming messages
    Given an IntelliJ project and a VS Code project connected to one Telegram relay
    When a Telegram user selects the VS Code project
    And the user sends a message to the bot
    Then only the VS Code project receives the message
    And both projects send replies through the relay
