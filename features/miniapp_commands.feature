@miniapps
Feature: Distill a shell command session into a command-profile Mini App
  A session where the agent ran one fixed external command (via run_command)
  becomes a Mini App whose step calls the operator-declared command profile.
  The profile travels inside the document, and execution never uses a shell.

  Scenario: An untrusted embedded profile pauses the run until the operator trusts it
    Given a command profile for the fake encoder is declared in the config
    And a completed session that ran the fake encoder over a media file
    When I start Mini App distillation for the command session
    And I confirm the selected command scenario
    And the config no longer declares the fake encoder profile
    And I test the command draft with its source inputs
    Then the test run pauses asking to trust the embedded profile
    When I approve the trust confirmation
    Then the command test run succeeds and the fake encoder ran without a shell
    And the trust approval is recorded on this machine

  Scenario: A run_command session becomes a released command-profile Mini App
    Given a command profile for the fake encoder is declared in the config
    And a completed session that ran the fake encoder over a media file
    When I start Mini App distillation for the command session
    And I confirm the selected command scenario
    Then the draft carries a command step and embeds its profile
    When I test the command draft with its source inputs
    Then the command test run succeeds and the fake encoder ran without a shell
    When I release the command draft as version "1.0.0"
    And I run released command version "1.0.0" with a different media file
    Then the released command run executed the fake encoder with the new file
