Feature: A streamed answer that goes silent is cut after a bounded idle time
  A provider that accepts the request and then stops sending mid-way leaves
  the turn waiting forever: nothing on the wire says the answer will never
  finish, and the first-token guard has already been satisfied. The stall
  guard (agent.llm_stream_idle_timeout_ms) cuts a stream that delivered
  nothing for that long after its first chunk and keeps the text already
  delivered next to a stall error that names the idle time.

  It counts the chunks the stream readers hand on - text, reasoning, and the
  progress frames a tool call's argument fragments become - and not the
  keep-alive comments or empty frames a gateway keeps sending for a model
  that is gone, so such a stream is still cut. It sits outside the provider's
  retry wrapper: what to do with a stall - carry the answer on, re-issue a
  call that showed nothing, wait out a silent provider - is the agent's call
  (features/llm_stall_retry.feature). A blocking (stream: false) answer
  arrives in one piece and is never guarded.

  Scenario: A stream that stalls after text deltas fails with a stall error and keeps the partial text
    Given an "openai" provider with a stream idle timeout of 200 ms pointed at a stub server that stalls after text deltas
    When a streaming completion is requested
    Then the call fails with a stall error that names the idle time
    And the partial response preserves text "Hello fr"
    And the stub server received 1 request

  Scenario: A stream that stalls before any text is cut and left to the agent
    Given an "openai" provider with a stream idle timeout of 200 ms whose upstream stalls once after an empty first frame and then streams a completion
    When a streaming completion is requested
    Then the call fails with a stall error that names the idle time
    And no partial response is kept
    And the stub server received 1 request

  Scenario: Keep-alive comments do not hold a dead stream open
    Given an "openai" provider with a stream idle timeout of 200 ms pointed at a stub server that sends keep-alive comments after text deltas
    When a streaming completion is requested
    Then the call fails with a stall error that names the idle time
    And the partial response preserves text "Hello fr"

  Scenario: A tool call whose arguments keep streaming is not cut
    Given an "openai" provider with a stream idle timeout of 200 ms pointed at a stub server that streams tool call arguments for 600 ms
    When a streaming completion is requested
    Then the call succeeds with one tool call
