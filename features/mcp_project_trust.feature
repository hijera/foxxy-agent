Feature: Project-local MCP servers need approval before they run
  A workspace can be someone else's checkout, and <workspace>/.foxxycode/mcp.json
  travels with it: the repository picks the command, its arguments, and its
  environment. FoxxyCode therefore treats project-local MCP entries as untrusted.
  Creating or restoring a session for the workspace does not start them until
  the operator approves that exact declaration for that workspace, and editing
  the declaration withdraws the approval. Entries the operator wrote
  themselves (config.yaml and the global ~/.foxxycode/mcp.json) are unaffected.

  @acp
  Scenario: Creating a session does not run an unapproved project server
    Given a workspace whose project mcp.json runs a marker command
    When an ACP client creates a session for that workspace
    Then the marker command has not run
    And foxxycode reports the project MCP server "marker" as awaiting approval

  @acp
  Scenario: An approved project server starts with the session
    Given a workspace whose project mcp.json runs a marker command
    And the operator approved the project MCP server "marker" for that workspace
    When an ACP client creates a session for that workspace
    Then the marker command has run

  @acp
  Scenario: Rewriting an approved declaration withdraws the approval
    Given a workspace whose project mcp.json runs a marker command
    And the operator approved the project MCP server "marker" for that workspace
    And the project mcp.json is rewritten to run a different marker command
    When an ACP client creates a session for that workspace
    Then the marker command has not run
    And foxxycode reports the project MCP server "marker" as awaiting approval

  @acp
  Scenario: A global server still starts without approval
    Given a workspace whose global mcp.json runs a marker command
    When an ACP client creates a session for that workspace
    Then the marker command has run

  # Fork-specific: FoxxyCode lets a live session change its workspace, which
  # rebuilds the configuration-derived MCP clients. That second entry point has
  # to pass the same gate, or switching into a repository would run its command.
  @acp
  Scenario: Switching the session workspace re-applies the gate
    Given a workspace whose project mcp.json runs a marker command
    And the operator approved the project MCP server "marker" for that workspace
    And a second workspace whose project mcp.json runs a marker command
    When an ACP client creates a session for the first workspace
    And the session switches to the second workspace
    Then the second marker command has not run
    And the second workspace reports the project MCP server "marker" as awaiting approval

  @acp
  Scenario: Switching into an approved workspace starts its server
    Given a workspace whose project mcp.json runs a marker command
    And a second workspace whose project mcp.json runs a marker command
    And the operator approved the project MCP server "marker" for the second workspace
    When an ACP client creates a session for the first workspace
    And the session switches to the second workspace
    Then the second marker command has run

  # Fork-specific: servers supplied by the ACP client come from the editor the
  # operator is running, not from the checkout, so the gate must not touch them.
  @acp
  Scenario: A server supplied by the ACP client is not gated
    Given an ACP client that supplies a marker MCP server
    When an ACP client creates a session for that workspace
    Then the marker command has run

  @http
  Scenario: The MCP list reports an unapproved project server instead of probing it
    Given a running foxxycode HTTP server
    And a project mcp.json defining the stdio server "demo"
    When I list the MCP servers
    Then the MCP list shows server "demo" as awaiting approval
    And server "demo" exposes no tools
    When I approve the MCP server "demo"
    And I list the MCP servers
    Then the MCP list shows server "demo" from source "local" as enabled
    And server "demo" exposes the tool "echo" as enabled
