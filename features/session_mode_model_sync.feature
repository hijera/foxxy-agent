Feature: The session mode and the backend model stay in sync with the SPA
  The composer's Mode and Model are session state, not client state. The gateway
  persists the mode across a restart, lets a client store it without sending a
  turn, and announces every switch it makes on its own — running a plan, or the
  model calling plan_exit — as an SSE frame, so a connected SPA does not keep
  posting the profile the session has already left. The YAML backend the client
  picks is the one that reaches the provider.

  Scenario: A session reopened after a restart is still in the mode it was left in
    Given a foxxycode gateway backed by a stub model
    When a client sends a "plan" prompt over POST /v1/responses
    And the gateway restarts
    Then the transcript reports the session in "plan" mode

  Scenario: A client stores the mode without sending a turn
    Given a foxxycode gateway backed by a stub model
    And a session created for the client
    When the client patches the session mode to "ask"
    Then the patch response echoes the "ask" mode
    And the gateway restarts
    And the transcript reports the session in "ask" mode

  Scenario: Running a plan announces the switch back to agent mode
    Given a foxxycode gateway backed by a stub model
    And a session created for the client
    And a design plan the client can run
    When the client runs that plan from "plan" mode
    Then the stream carries a mode frame naming "agent"
    And the transcript reports the session in "agent" mode

  Scenario: plan_exit announces the switch back to agent mode
    Given a foxxycode gateway backed by a stub model that leaves plan mode
    When a client sends a "plan" prompt over POST /v1/responses
    Then the stream carries a mode frame naming "agent"
    And the transcript reports the session in "agent" mode

  Scenario: The backend model the client picks is the one that reaches the provider
    Given a foxxycode gateway backed by a stub model
    When a client sends an "agent" prompt selecting the "local/alt" backend
    Then the provider received the request as "local/alt"
    And the transcript reports "local/alt" as the session model override
