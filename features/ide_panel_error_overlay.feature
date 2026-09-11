Feature: The embedded IDE panel's error overlay reports bugs, not dropped requests
  The IntelliJ and VS Code panels inject a bootstrap script that paints a red
  "FoxxyCode UI error" bar over the bottom of the chat for anything uncaught, so
  the SPA never silently goes blank. The bar has to be reserved for actual bugs.

  While a turn runs the SPA polls the backend on 127.0.0.1 several times a second,
  so one refused or reset connection is routine — and a "TypeError: Failed to fetch"
  from such a poll used to paint the bar permanently, because the benign-message
  filter guarded only the "error" event and never the rejection path. A real outage
  is reported by the SPA's own offline indicators and, on IntelliJ, by a notification
  when the backend process exits, so the overlay can ignore transport failures.

  The bar is also dismissible now: before, nothing but reloading the panel removed
  it, which is what turned a survivable hiccup into a chat the user could not read.

  Background:
    Given the IntelliJ panel bootstrap script is installed on a blank page

  Scenario: A dropped background request does not paint the overlay
    When a background request rejects with "Failed to fetch"
    Then the error overlay stays hidden

  Scenario: A request the page itself cancelled does not paint the overlay
    When a request is aborted by the page
    Then the error overlay stays hidden

  Scenario: A genuine bug still paints the overlay, and can be dismissed
    When a promise rejects with an uncaught "boom"
    Then the error overlay reports "boom"
    And the error overlay can be dismissed
