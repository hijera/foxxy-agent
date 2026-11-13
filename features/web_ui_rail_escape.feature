Feature: Escape closes the screen the rail opened
  Every item of the rail but Sign out opens a screen over the chat: History,
  Scheduler, Swarm, Docs and Settings. Escape closes the one on screen the way
  its close control does, and the address goes back to the chat. It undoes one
  step at a time: a menu, a picker, a tip or a search box with text in it takes
  the key before the screen does, and a screen with a step of its own - an open
  job of the scheduler, an open row of Settings, a section of Settings on the
  stacked shell - takes that step first. The rule is written once for every
  screen of the rail, so a screen added to the rail later closes on Escape
  without a handler of its own. Before, only History and the scheduler listened
  for the key, and History left its address behind.

  Scenario: The documentation reader and Settings close on Escape
    Then Escape closes the documentation reader opened from the rail
    And Escape closes Settings opened from the rail

  Scenario: Every screen of the rail closes on Escape, one added later included
    Then every screen of the rail closes on Escape and gives the address back to the chat
    And a screen added to the rail closes by the same rule

  Scenario: Escape undoes one step at a time
    Then the scheduler leaves an open job for its list first
    And Settings leaves an open row for its list first
    And on the stacked shell Settings goes back to its tiles first
    And a search with text in it is cleared before the reader closes
    And a row menu of History folds before History closes
