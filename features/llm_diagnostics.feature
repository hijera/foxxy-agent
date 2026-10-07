Feature: LLM connection diagnostics
  Scenario: Debugging the first model response
    Given LLM debug logging is enabled
    When a model streams a response over HTTP
    Then the log identifies network stages and the first model output without message contents
