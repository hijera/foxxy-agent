Feature: A session gets its title even when the turn does not finish
  The sidebar title is generated from the user's first message, not from the
  assistant's answer, so it does not depend on the turn producing anything.
  A turn the user stops mid-stream must therefore still end up titled: the
  question was asked, and that is all the title needs.

  Scenario: A completed turn titles the session
    Given a fresh session
    When the user asks "how do I connect postgres to my API"
    And the model answers normally
    Then the session title is generated from the user's message
    And the title is broadcast to clients once

  Scenario: A turn stopped mid-answer still titles the session
    Given a fresh session
    When the user asks "how do I connect postgres to my API"
    And the user stops the turn while the model is still writing
    Then the turn ends as cancelled
    And the partial answer is kept in the transcript
    And the session title is generated from the user's message
    And the title is broadcast to clients once

  Scenario: A turn stopped before the model writes anything still titles the session
    Given a fresh session
    When the user asks "how do I connect postgres to my API"
    And the user stops the turn before the model writes anything
    Then the session title is generated from the user's message

  Scenario: A stopped turn never overwrites a title the user pinned
    Given a fresh session with the title pinned to "My own title"
    When the user asks "how do I connect postgres to my API"
    And the user stops the turn while the model is still writing
    Then the session title stays "My own title"
