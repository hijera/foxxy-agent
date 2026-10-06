Feature: Debug mode
  Debug mode diagnoses and fixes software issues. It has full tool access like
  Agent mode, but a methodology-driven system prompt: hypothesize 5-7 sources,
  narrow to 1-2, validate with logs or a focused run, and confirm the diagnosis
  with the user before applying a fix.

  Scenario: Debug exposes the full toolset and a diagnosis-first prompt
    Given a Debug-mode session with a responding model
    When the user reports a failing test
    Then the Debug prompt enforces the diagnosis methodology
    And the model receives full tool access including file mutation tools
    And the Debug answer is saved in the transcript
