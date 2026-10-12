Feature: Mention picker before the first request
  Files and folders can be selected without hiding the draft in an editor panel.

  Scenario Outline: Mention candidates stay above the composer
    Given an <state> composer at <width> pixels in the <theme> theme
    When I open the mention picker
    Then the mention picker stays above the composer and inside the viewport
    And I can click a folder and keep its mention in the draft
    And I can click a file inside that folder

    Examples:
      | state  | width | theme |
      | empty  | 390   | dark  |
      | empty  | 630   | light |
      | empty  | 1280  | dark  |
      | docked | 390   | light |
      | docked | 630   | dark  |
      | docked | 1280  | light |
