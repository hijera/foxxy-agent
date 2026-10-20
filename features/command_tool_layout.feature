Feature: Command tool card layout
  Command input and output remain readable at every panel width.

  Scenario Outline: Short and long commands share a readable execution card
    Given command tool cards at <width> pixels in the <theme> theme
    When I expand the command tool cards
    Then command output starts below its argument preview
    And short commands need no overflow toggle
    And long commands can be expanded, scrolled and collapsed
    And long output can be expanded, scrolled and collapsed
    And command cards fit within the panel
    And shell action labels remain readable beside long commands

    Examples:
      | width | theme |
      | 390   | dark  |
      | 1280  | dark  |
      | 390   | light |
      | 1280  | light |
