Feature: Mention picker before the first request
  Files and folders can be selected without hiding the draft in an editor panel.

  Scenario Outline: Mention candidates stay above the composer
    Given an <state> composer at <width> by <height> pixels in the <theme> theme
    When I open the mention picker
    Then the mention picker stays above the composer and inside the viewport
    And I can click a folder and keep its mention in the draft
    And I can click a file inside that folder

    Examples:
      | state  | width | height | theme |
      | empty  | 390   | 900    | dark  |
      | empty  | 630   | 900    | light |
      | empty  | 1280  | 900    | dark  |
      | docked | 390   | 900    | light |
      | docked | 630   | 900    | dark  |
      | docked | 1280  | 900    | light |
      | empty  | 390   | 420    | dark  |
      | empty  | 630   | 420    | light |
      | empty  | 1280  | 360    | dark  |
      | docked | 630   | 420    | dark  |

  Scenario Outline: An open list shrinks with the editor window
    Given an empty composer at <width> by 900 pixels in the dark theme
    When I open the mention picker
    And I reduce the window height to <height> pixels
    Then the mention picker stays above the composer and inside the viewport
    And I can scroll the list to its last candidate without scrolling the page
    And I can click a folder and keep its mention in the draft

    Examples:
      | width | height |
      | 390   | 420    |
      | 630   | 420    |
      | 1280  | 360    |
