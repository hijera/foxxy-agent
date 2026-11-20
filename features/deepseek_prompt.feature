Feature: DeepSeek prompt selection

  Scenario Outline: A built-in mode keeps its provider guidance when DeepSeek has no own guidance
    Given a DeepSeek model served by "openai"
    When the "<mode>" system prompt is built for DeepSeek
    Then the DeepSeek system prompt includes "<guidance>"

    Examples:
      | mode  | guidance                        |
      | agent | Model-family notes (DeepSeek)   |
      | ask   | Model-family notes (OpenAI / GPT) |
      | plan  | OpenAI API planning prompt       |

  Scenario Outline: An existing custom provider prompt still applies to DeepSeek
    Given a DeepSeek model served by "<provider>"
    And the custom DeepSeek prompt directory has "agent.md" with "shared prompt"
    And the custom DeepSeek prompt directory has "agent.<provider>.md" with "provider prompt"
    When the "agent" system prompt is built for DeepSeek
    Then the DeepSeek system prompt includes "provider prompt"

    Examples:
      | provider   |
      | openai     |
      | neuraldeep |

  Scenario: A custom DeepSeek prompt takes precedence over a provider prompt
    Given a DeepSeek model served by "openai"
    And the custom DeepSeek prompt directory has "agent.openai.md" with "provider prompt"
    And the custom DeepSeek prompt directory has "agent.deepseek.md" with "DeepSeek prompt"
    When the "agent" system prompt is built for DeepSeek
    Then the DeepSeek system prompt includes "DeepSeek prompt"
