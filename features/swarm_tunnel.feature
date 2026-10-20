Feature: Reverse swarm tunnel
  A node without an inbound listening port can serve requests over its outbound connection.

  Scenario: A freshly initialized relay forwards a request through a tunnel
    Given a fresh relay and a node connected only by an outbound tunnel
    When a client requests the node sessions through the relay
    Then the tunneled node receives the sessions request
