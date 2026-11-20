Feature: Kimi prompt selection and reasoning support
  Kimi instructions follow the model family, while reasoning controls follow
  the capabilities of the selected model.

  Scenario Outline: OpenAI-compatible Kimi models keep their own capabilities
    Given an OpenAI-compatible Kimi model "<model>"
    When the Kimi agent system prompt is built
    Then the prompt includes Kimi guidance and shared file-search guidance
    And the model offers reasoning levels "<levels>"

    Examples:
      | model                            | levels          |
      | kimi-k2.6                        | low,medium,high |
      | moonshotai/Kimi-K2-Thinking      | low,medium,high |
      | moonshotai/Kimi-K2-Instruct      | none            |
      | moonshotai/Kimi-K2-Instruct-0905 | none            |
      | moonshot-v1-128k                 | none            |
