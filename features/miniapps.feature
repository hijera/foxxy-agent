@miniapps
Feature: Distill and run a reusable mini app
  A web operator can turn a successful session into an editable workflow,
  verify it with the same data, release an immutable version, and run it again.

  Scenario: Distill, test, release, and run a mini app
    Given a completed FoxxyCode session about formatting a greeting
    When I distill the session into a mini app
    Then an editable mini app draft is created
    When I replace the draft with a deterministic greeting workflow
    And I generate an expected result for "The greeting must address the supplied person"
    Then the draft contains a model-verified expected result
    And I test the draft with the name "Foxxy"
    Then the test run succeeds with the text "Hello, Foxxy!"
    When I release the tested draft
    And I run released version "1.0.0" with the name "Operator"
    Then the released run succeeds with the text "Hello, Operator!"
