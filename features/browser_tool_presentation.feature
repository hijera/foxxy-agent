Feature: Browser tool presentation in the transcript
  Scenario: Read browser actions and their results
    Given a transcript containing browser tool calls
    When I expand the browser calls
    Then navigation has a readable action heading
    And the screenshot result can be enlarged
    And JavaScript is formatted and highlighted
    And long code can be expanded and collapsed
    And scroll offsets appear inside a screen diagram
    And text results and page log errors remain visible
