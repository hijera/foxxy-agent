Feature: Start-screen composer placement
  The title and prompt field occupy the middle of an empty editor panel.
  Project links have their own space below the field, including short panels.

  Scenario Outline: The empty panel keeps its composer clear of the footer
    Given an empty editor panel measuring <width> by <height> pixels
    Then the title and composer are vertically centered
    And the project footer is below the composer without horizontal overflow

    Examples:
      | width | height |
      | 640   | 690    |
      | 390   | 700    |
      | 320   | 480    |
      | 1280  | 900    |
