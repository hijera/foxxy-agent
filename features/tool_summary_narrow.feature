Feature: Tool summaries in a narrow chat panel
  A long MCP action label must leave room for the duration without widening the page.

  Scenario Outline: Long tool summaries fit the panel
    Given tool summaries in "<locale>" at <width> pixels
    Then the tool summaries do not widen the page
    And every tool duration stays visible inside its summary

    Examples:
      | locale | width |
      | en     | 303   |
      | ru     | 303   |
      | en     | 390   |
      | en     | 1280  |
