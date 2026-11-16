Feature: Shared file actions remain reachable
  A file card clips its thumbnail but must not clip the menu beside it.

  Scenario: Open and dismiss a file card's actions
    Then file actions escape card clipping and close with Escape

  Scenario: Toggle file actions with the pointer
    Then clicking file actions a second time closes the menu

  Scenario: Reveal a shared file on the selected server
    Then file actions send an authenticated POST to reveal the source on its server

  Scenario: Copy from a menu inside an embedded browser
    Then file names and paths still copy when Clipboard API writes are denied
