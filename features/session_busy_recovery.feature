Feature: Recover a long-running turn after the IDE panel reloads
  A long agent turn keeps running on the server when the editor webview reloads,
  and it keeps holding the session turn lock. The panel must be able to find that
  turn again and keep rendering it, and a send into the busy chat must name the
  chat that is still working instead of dead-ending on an opaque error.

  Background:
    Given a running foxxycode HTTP server
    And a long agent turn in flight for my chat
    And the turn has already produced some output

  Scenario: The panel re-attaches to the turn it left behind
    When the IDE panel reloads
    Then my chat is reported as still working
    When the panel re-attaches to the live turn
    Then it replays the output produced before the reload
    When the turn finishes
    Then the re-attached panel receives the rest of the answer

  Scenario: Sending into the busy chat names the chat that is working
    When the IDE panel reloads
    And I send another message to that chat
    Then the send is refused as busy and names the running chat

  Scenario: A chat with no turn in flight says so immediately
    When the turn finishes
    And the panel re-attaches to the live turn
    Then it reports that there is no live turn without waiting

  Scenario: Stopping a slow model and retrying on another one
    When I stop the generation
    And I ask again with another model
    Then the new request is accepted
