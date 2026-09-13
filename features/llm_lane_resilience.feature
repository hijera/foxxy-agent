Feature: Recovering a turn the model lane, not the model, spoiled
  One model name at a proxy is often a group of interchangeable deployments, and
  a single sick member answers with internal reasoning and neither text nor a
  tool call - the harmony tool call flattened into reasoning_content, the
  function name lost. That failure belongs to the attempt, not to the
  conversation, so the loop re-issues the very same request once before it starts
  arguing with the model in words: the replay lands on a different member of the
  group and comes back with the call the first one dropped.

  The empty turn stays in the transcript, because the user watched its reasoning
  stream in, but the request that goes out again must not carry it - otherwise it
  is a different request, and the member that would have answered never sees the
  one that failed.

  A call that answers nothing at all is the neighbouring failure, and it has its
  own recoveries in features/llm_stall_retry.feature.

  Scenario: An empty assistant turn is re-issued before the model is nudged
    Given a model that answers with reasoning only on its first call and then replies
    When the operator sends a prompt on the lane
    Then the lane receives the same request a second time
    And the retried request carries no nudge and no empty assistant turn
    And the turn ends with the model's answer

  Scenario: The empty turn the user watched stays in the transcript
    Given a model that answers with reasoning only on its first call and then replies
    When the operator sends a prompt on the lane
    Then the transcript holds the empty turn as well as the answer

  Scenario: Words come only after a replay did not help
    Given a model that answers with reasoning only on every call
    When the operator sends a prompt on the lane
    Then the lane receives the same request a second time
    And the third request tells the model its previous message had no answer
