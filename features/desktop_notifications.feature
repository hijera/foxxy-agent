Feature: Native desktop notification content
  Scenario: A permission alert includes the chat, action buttons and silent audio
    Given a silent permission notification for chat "spotifySync"
    When the native notification payload is built
    Then the native notification names chat "spotifySync"
    And the native notification offers "Allow" and "Reject"
    And the native notification is silent

  Scenario: Alerts are shown only while FoxxyCode is in the background
    Given FoxxyCode is the foreground window
    Then a new desktop notification is suppressed
    When another application becomes the foreground window
    Then a new desktop notification is allowed

  Scenario: Foreground alerts can be enabled in settings
    Given FoxxyCode is the foreground window
    And notifications in the active window are enabled
    Then a new desktop notification is allowed
