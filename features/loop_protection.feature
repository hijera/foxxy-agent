Feature: Runaway loop protection
  A model that stops making progress — repeating the same passage inside one
  streamed response, requesting the same tool call with the same arguments over
  and over, or rotating through a fixed sequence of calls — is nudged once to
  change course. If it keeps looping, the looping calls are taken away for the
  rest of the turn while the turn itself runs on to a real answer: by then the
  model has usually gathered most of what it needs, and throwing that away costs
  the user more than the loop did. A model with nothing left but the loop is
  asked for its answer with the tools withheld. Setting agent.loop_stuck_action
  to "stop" restores the older behaviour of ending the turn with a notice. The
  repeated text is dropped from the transcript, so it is never replayed to the
  model and cannot re-seed the loop.

  Scenario: A degenerating answer stream is cut and the turn recovers
    Given a foxxycode agent with the loop guard enabled
    And a model that repeats the same sentence forever, then answers normally
    When the user sends a prompt
    Then the streamed response is cut before the model stops on its own
    And the stored transcript keeps the answer without the repeated tail
    And the model is nudged once to stop repeating itself
    And the nudged request carries no repeated passage
    And the turn ends with the model's real answer

  Scenario: A looping reasoning channel is cut the same way
    Given a foxxycode agent with the loop guard enabled
    And a model that repeats the same thought forever with no answer text, then answers normally
    When the user sends a prompt
    Then the streamed response is cut before the model stops on its own
    And the stored transcript keeps the reasoning without the repeated tail
    And the turn ends with the model's real answer

  Scenario: Identical tool calls stop the turn when the guard is set to stop
    Given a foxxycode agent whose loop guard is set to stop the turn
    And a model that always requests the same tool call with the same arguments
    When the user sends a prompt
    Then the tool is executed fewer times than the repeat limit allows
    And every requested tool call has a result recorded
    And the turn stops with a loop notice before max turns is reached

  Scenario: A rotating sequence stops the turn when the guard is set to stop, even when one call varies
    Given a foxxycode agent whose loop guard is set to stop the turn
    And a model that cycles through the same three tool calls, varying one of them
    When the user sends a prompt
    Then the turn stops with a loop notice before max turns is reached
    And the notice names a repeating sequence rather than one repeated call
    And every requested tool call has a result recorded

  Scenario: A quarantined loop leaves the turn free to finish
    Given a foxxycode agent with the loop guard enabled
    And a model that cycles through the same three tool calls, then answers once they stop running
    When the user sends a prompt
    Then the looping calls stop being executed
    And every requested tool call has a result recorded
    And the turn ends with the model's real answer

  Scenario: A model with nothing left but the loop is asked to answer
    Given a foxxycode agent with the loop guard enabled
    And a model that cycles through the same three tool calls, varying one of them
    When the user sends a prompt
    Then the looping calls stop being executed
    And the model is asked for an answer with the tools withheld
    And the turn ends without an error
    And every requested tool call has a result recorded
