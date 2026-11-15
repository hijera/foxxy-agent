Feature: Mini Apps verify command results using recorded evidence
  Scenario: Verify a file converter whose assistant described the result in prose
    Given a successful file command with a conversational final answer
    When I create a Mini App from the command session and test the source inputs
    Then its source verification passes using the recorded command result

  Scenario: Verify a calculator whose assistant explained the numeric result
    Given a successful scalar command with a conversational final answer
    When I create a Mini App from the command session and test the source inputs
    Then its source verification passes using the recorded command result
