Feature: Embedded IDE panel opens a transcript without a ResizeObserver loop
  The IntelliJ and VS Code panels host the SPA in an embedded Chromium (JCEF is
  Chromium 104) and paint a red "FoxxyCode UI error" overlay over the chat for
  any uncaught window error. Opening an old transcript raised
  "ResizeObserver loop limit exceeded" there, for two reasons that current
  browsers tolerate: transcript rows used content-visibility: auto with a
  placeholder size, which that engine flips between the placeholder and the
  real box inside its own resize-observation loop, and the observer that keeps
  the scroll tail as tall as the composer wrote its measurement back into the
  transcript from inside that same loop. The IDE embed now lays rows out
  plainly (html[data-embed] switches the optimization off), the reserve write
  waits for the next animation frame everywhere, and the overlay treats the
  browser's loop notice as the deferral it is, not as an error of the SPA.
  The Chromium 104 half is guarded by the plugin's own uiTest
  (BrowserPanelUiTest.anOldTranscriptOpensWithoutAResizeObserverLoop); the
  scenarios below run the same page in a current Chrome.

  Background:
    Given a foxxycode HTTP server with a transcript of 60 tool-call exchanges

  Scenario: An old transcript opens in the embedded panel at a narrow width
    When the embedded panel opens that transcript at 360 pixels wide
    And the user drafts a message and the composer grows taller
    Then the transcript and the composer are rendered
    And no ResizeObserver loop error was raised
    And every composer reserve write landed in a later frame than the resize observation that measured it

  Scenario: The panel's error overlay ignores the browser's resize-loop notice
    Given the IntelliJ panel bootstrap script is installed on a blank page
    When the page raises "ResizeObserver loop limit exceeded"
    And the page raises "ResizeObserver loop completed with undelivered notifications."
    Then the error overlay stays hidden
    When the page raises an uncaught "TypeError: boom"
    Then the error overlay reports "boom"
