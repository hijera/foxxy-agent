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

  Scenario: An operator approves a checkpoint before the workflow acts
    Given a completed FoxxyCode session that wrote a greeting file
    When I start Mini App distillation for the session
    And I confirm the selected greeting-file scenario
    And I add a confirmation checkpoint to the generated draft
    And I start a test run of the draft
    Then the test run waits for my approval
    When I approve the pending confirmation
    Then the test run finishes successfully

  Scenario: A released Mini App keeps its catalog entry and run history
    Given a completed FoxxyCode session that wrote a greeting file
    When I start Mini App distillation for the session
    And I confirm the selected greeting-file scenario
    And I test the unchanged generated draft with its source inputs
    And I release the tested draft as version "1.0.0"
    And I run released version "1.0.0" with a different greeting
    Then the catalog lists the Mini App as released at version "1.0.0"
    And the run history for the Mini App lists that run

  Scenario: A second release supersedes the first without replacing it
    Given a completed FoxxyCode session that wrote a greeting file
    When I start Mini App distillation for the session
    And I confirm the selected greeting-file scenario
    And I test the unchanged generated draft with its source inputs
    And I release the tested draft as version "1.0.0"
    And I retest and release the draft as version "1.1.0"
    Then both released versions stay retrievable
    And the catalog lists the Mini App as released at version "1.1.0"
