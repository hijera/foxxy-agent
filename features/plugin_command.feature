Feature: Plugin and marketplace management via the /plugin command
  The built-in /plugin command manages skill plugins and their marketplaces from
  chat, mirroring the `foxxycode plugin ...` CLI. It runs deterministically without
  an LLM turn and works over the HTTP prompt surface (/v1/responses). Adding a
  marketplace reads its plugin list and installs nothing; a plugin of it is
  installed by name, as <plugin>@<marketplace>.

  Background:
    Given a running foxxycode plugin server
    And a chat session
    And a local marketplace "shop" publishing skill "demo" at version "1.0.0"

  Scenario: Add a marketplace from chat, then install one of its plugins
    When I send the plugin prompt "/plugin marketplace add <shop>"
    Then the plugin response mentions "Added marketplace"
    And the plugin response mentions "1 plugin(s)"
    And the "/plugin" command is part of the transcript
    When I send the plugin prompt "/plugin install demo@shop"
    Then the plugin response mentions "Installed demo@shop. 1 added, 0 updated, 0 failed."

  Scenario: List marketplaces reports validity status
    Given I have added the marketplace "shop" over chat
    When I send the plugin prompt "/plugin marketplace list"
    Then the plugin response mentions "valid marketplace"
    And the plugin response mentions "shop"

  Scenario: List installed plugins shows versions
    Given I have added the marketplace "shop" over chat
    And I have sent the plugin prompt "/plugin install demo@shop"
    When I send the plugin prompt "/plugin list"
    Then the plugin response mentions "demo@1.0.0"

  Scenario: Remove a marketplace from chat
    Given I have added the marketplace "shop" over chat
    When I send the plugin prompt "/plugin marketplace remove <shop>"
    Then the plugin response mentions "Removed marketplace"
