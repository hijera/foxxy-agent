@miniapps
Feature: Distill and run a reusable Mini App
  A web operator can turn a successful tool-driven session into an editable,
  verified, and immutable workflow without replacing the generated workflow.

  Scenario: A successful file task becomes a released Mini App
    Given a completed FoxxyCode session that wrote a greeting file
    When I start Mini App distillation for the session
    And I confirm the selected greeting-file scenario
    Then the generated draft contains inferred inputs and a write-file tool step
    When I test the unchanged generated draft with its source inputs
    Then the test run reproduces the accepted greeting file
    When I release the tested draft as version "1.0.0"
    And I run released version "1.0.0" with a different greeting
    Then the released run writes the different greeting
