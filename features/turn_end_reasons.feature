Feature: A turn ends for a reason the user can see, and a sick provider does not end it
  A turn that stops before the task is done has to say why (upstream issue #255).
  A turn cut by its step limit, agent.max_turns, ends with a notice that names
  the limit: streamed into the answer, kept in the transcript next to it, and
  handed to the caller that reads the result rather than the stream.
  FoxxyCode keeps 30 steps as the limit when none is set; upstream reads an
  unset limit as none at all.

  A provider that breaks in the middle of an answer - a 500 from a proxy whose
  fallback failed, a connection cut - is a failure of the lane, not of the
  conversation (upstream issue #246). The turn keeps the part of the answer
  the user already watched stream in, parks for a pause it announces, and asks
  the model to go on from there, instead of ending and leaving the user to
  type "continue". It spends the turn's continuation budget, as a stall does,
  and agent.llm_continue turns it off.

  Scenario: Without a step limit configured a turn stops after 30 steps and says so
    Given a model that reads a file 35 times before it answers
    And an agent with no step limit configured
    When the user sends a prompt
    Then the turn stops at its step limit
    And the transcript ends with a notice that names agent.max_turns and 30 steps
    And the prompt result carries the same notice

  Scenario: A turn stopped by its step limit says so
    Given a model that reads a file 35 times before it answers
    And an agent whose agent.max_turns is 3
    When the user sends a prompt
    Then the turn stops at its step limit
    And the transcript ends with a notice that names agent.max_turns and 3 steps
    And the prompt result carries the same notice
    And the session's UI log carries no copy of it

  Scenario: A provider that fails mid-answer does not end the turn
    Given a model whose first answer breaks off with "server error 500" after "The project holds"
    And an agent with no step limit configured
    When the user sends a prompt
    Then the turn ends with the model's answer
    And the transcript keeps "The project holds" that the user already saw
    And the next request carried that part and asked the model to continue
    And the turn announced the pause before it went on

  Scenario: With agent.llm_continue off a provider that fails mid-answer ends the turn
    Given a model whose first answer breaks off with "server error 500" after "The project holds"
    And an agent that does not carry a cut answer on
    When the user sends a prompt
    Then the turn fails with "server error 500"
    And the model was called 1 times
