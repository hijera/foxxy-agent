Feature: A provider that goes quiet is waited out, not surrendered to
  A saturated gateway fails in two ways, and both used to end the turn with a red
  SYSTEM row that the operator had to restart by hand with "continue".

  A call can answer nothing at all - silence, a dropped connection, a
  provider-side timeout, a 5xx. Nothing reached the transcript, so FoxxyCode
  pauses and re-issues the identical request: one minute, then three, then five,
  and five for every attempt after that, up to an hour of waiting in total.

  Or a stream can deliver thousands of frames and then stop mid-answer, with no
  finish_reason, no [DONE] and the connection held open. There the partial answer
  is kept and the model is asked to carry on from it, because replaying would
  show the same text twice.

  A model asked to carry on does not always obey: it starts the same answer from
  the top instead, the connection eats that one too, and the operator watches the
  same opening paragraph arrive every few minutes. So the attempts are compared
  with each other. The second time an opening comes back, the model is told that
  it is repeating itself and which steps have already run; if it keeps going, the
  tools come off for one request and it has to answer from what it gathered.

  Scenario: A silent call is retried after a pause and the turn completes
    Given a model that answers nothing on its first call and then replies
    When the operator sends a prompt
    Then the turn completes without a system error
    And the model was called twice
    And the transcript holds exactly one answer

  Scenario: A dropped connection is waited out like silence
    Given a model whose first call dies with a transport error and then replies
    When the operator sends a prompt
    Then the turn completes without a system error
    And the model was called twice

  Scenario: Waiting for a silent provider does not spend the reasoning budget
    Given a model that answers nothing on its first call and then replies
    And the turn budget allows only one reasoning step
    When the operator sends a prompt
    Then the turn completes without a system error

  Scenario: A stream that stops mid-answer keeps its text and is continued
    Given a model that stops mid-answer on its first call and then finishes
    When the operator sends a prompt
    Then the turn completes without a system error
    And the partial answer survives in the transcript
    And the second request asked the model to continue

  Scenario: A model streaming one large tool call is never cut
    Given a model that spends longer than the stall guard writing one tool call
    When the operator sends a prompt
    Then the turn completes without a system error
    And the model was called once

  Scenario: A model that keeps restarting the same answer is told so
    Given a model that stalls on the same opening every time
    When the operator sends a prompt
    Then the first continuation asks the model to carry on
    And the second continuation tells the model it is repeating itself
    And the continuation nudges stay out of the transcript
    And the turn stops with a notice naming the restarts

  Scenario: A stall with nothing but reasoning is not asked to continue from an empty message
    Given a model that stalls after thinking but before writing, then answers
    When the operator sends a prompt
    Then the turn completes without a system error
    And the continuation does not point at an empty message

  Scenario: A session reopened after a restart is not invited to repeat itself
    Given a session whose previous turn ended writing the same answer twice
    When the operator sends a prompt
    Then the first request tells the model its previous turn was repeating
    And the first request names the steps that already ran
    And the continuation nudges stay out of the transcript
