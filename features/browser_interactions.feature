Feature: The browser drives real user interactions
  The first browser tool set could click, fill and look, but a real page needs
  more: a form submits on Enter, content arrives late, a dropdown must be picked
  by its label, a confirm() blocks the page, a form has five fields, an API must
  be forced into an error state, and links open new tabs. These tools close those
  gaps: press_key, wait_for, select_option, handle_dialog, fill_form, find,
  route mocks, storage writes and tab management.

  Background:
    Given a browser session on the interactions page

  Scenario: Pressing Enter submits a form the way a user would
    When I fill the search field and press Enter in it
    Then the page shows what was submitted

  Scenario: Waiting for late content instead of polling
    When I wait for the late content text
    Then the wait reports the condition was met

  Scenario: A dropdown is listed and picked by its visible label
    When I list the options of the pet dropdown
    Then the listing names the Cat and Dog options
    When I pick the Dog option by label
    Then the page's change handler saw the dog value

  Scenario: A confirm dialog is answered as armed
    When I arm the next dialog to be accepted
    And I click the button that asks for confirmation
    Then the page shows the accepted verdict

  Scenario: An unarmed dialog is dismissed so nothing hangs
    When I click the button that asks for confirmation
    Then the page shows the dismissed verdict
    And the page log records the dialog

  Scenario: Several fields are filled in one call
    When I fill the whole form in one call
    Then every field holds its value

  Scenario: A mocked API response reaches the page
    When I mock the data API with a canned answer
    And I navigate to the data API path
    Then the page shows the canned answer

  Scenario: A second tab is opened, switched to and closed
    When I open the second page in a new tab
    Then the tab list shows two tabs
    When I switch back to the first tab
    Then the current page is the first page
