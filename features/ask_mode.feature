Feature: Ask mode
  Ask mode answers repository questions without changing the workspace.
  It may research with explicitly read-only tools, but mutating tools are not
  exposed to the model.

  Scenario: Ask answers a repository question with read-only tools
    Given an Ask-mode session with a responding model
    When the user asks how the repository works
    Then the Ask prompt enforces the read-only boundary
    And the model receives repository, shell, and web research tools
    And the model receives no file mutation tools
    And the Ask answer is saved in the transcript
