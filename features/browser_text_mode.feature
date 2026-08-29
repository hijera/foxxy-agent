Feature: The browser can be driven without screenshots
  The interactive browser answers with a screenshot after every action, which is
  useless to a model that cannot see images and costs about 40 KB of base64 on every
  request that can. Two tools read the page as text instead: one outlines its
  structure and the selectors to act on, the other reports what the page said about
  itself. A config switch turns the images off entirely.

  Background:
    Given a browser session on a page that logs an error, throws, requests a URL that fails, and writes to every store

  Scenario: The page outline names the controls and how to reach them
    When I read the page as text
    Then the outline names the heading and the button
    And the outline carries a selector for the search field
    And no screenshot was taken

  Scenario: A selector from the outline can be used to act on the page
    When I read the page as text
    And I fill the search field using the selector the outline reported
    Then the outline reports the value I typed

  Scenario: The page log explains a failure a screenshot would not show
    When I read the page log until it reports everything
    Then the page log reports the console error
    And the page log reports the uncaught exception with its message
    And the page log reports the failed request and its status
    And no screenshot was taken

  Scenario: Screenshots can be turned off entirely
    Given the browser is configured with screenshots off
    When I navigate to the page
    Then no image is handed to the model
    And the answer still reports the page URL
    And the answer says screenshots are disabled

  # What the app has stored decides how it behaves, and none of it is visible in a
  # picture. Long values are cut: seeing that a token is set is the point, pasting
  # a whole JWT into every look is not.
  Scenario: Stored state is reported, with long values truncated
    When I inspect the page storage
    Then the report names every store the page wrote
    And a long stored value is truncated
    And no screenshot was taken

  Scenario: Load timing answers why a page is slow
    When I inspect the page timing
    Then the report breaks the load into phases
    And the report names the slowest requests

  # performance.memory is Chrome-only and absent in some configurations. Reporting
  # zeroes there would read like a measurement, so the tool has to say it cannot see.
  Scenario: Page weight is reported, and an unavailable heap is admitted
    When I inspect the page memory
    Then the report counts the DOM nodes
    And the report either sizes the JS heap or says it is unavailable
