Feature: MCP startup failures keep configured credentials out of logs
  Scenario: A global server echoes its credentials while refusing initialization
    Given an MCP server that echoes configured credentials in its startup error
    When the server pool keeps that global MCP server
    Then the backend startup warning masks those credentials
    And the startup warning keeps the HTTP status readable
