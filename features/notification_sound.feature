Feature: A custom notification sound
  Scenario: Select an audio file and restore the standard sound
    Given FoxxyCode has no custom notification sound
    When I upload a WAV notification sound and select it in settings
    Then the selected audio is available for notification playback
    When I restore the standard notification sound
    Then no custom audio is selected
