Feature: The menus of the web UI stay with what they serve
  A menu belongs to the control that opened it and to the draft it completes.
  Typing a slash command opens the completion menu over the composer; sending
  that draft with the Send button, or queueing it while a turn runs, empties the
  composer from outside, and the textarea fires no change event for a value its
  parent set, so the menu stayed over the empty composer until the next
  keystroke. On a relay the environment chip is the last thing on the right of
  the swarm header, and its menu, hung from the chip's left edge, ran past the
  window at 1280 px, cutting off the entries a token is added with.

  Scenario: Sending or queueing a command closes its menu
    Then Send clicked while the slash menu is open closes the menu
    And a draft queued while the slash menu is open closes the menu

  Scenario: The environment menu stays inside the window
    Then the environment menu of a chip near the right edge stays inside the window
