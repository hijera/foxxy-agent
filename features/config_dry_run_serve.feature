Feature: foxxycode serve --dry-run and foxxycode http --dry-run check the addresses they would bind
  A daemon that fails to bind its port dies in a log file nobody is tailing yet.
  foxxycode serve --dry-run resolves the subsystems the configuration enables exactly as
  a start would, flags included, and tries each listen address once, so a taken port
  is reported in the terminal that typed the command, next to the line that set it.
  foxxycode http - the command the editor plugins start - does the same for the one
  address it binds.

  Scenario: the HTTP listen address is checked before the daemon starts
    Given a config.yaml with the HTTP API on a free port and a provider that answers
    When I run foxxycode serve with --dry-run --test-config
    Then the command succeeds
    And the report marks httpserver as ok mentioning "free"

  Scenario: a port another process holds is reported at httpserver.port
    Given a config.yaml with the HTTP API on a port another process holds
    When I run foxxycode serve with --dry-run
    Then the command fails
    And the report marks httpserver as an error mentioning "in use"
    And the report points at the line of "port:"

  Scenario: foxxycode http checks its listen address before it starts
    Given a config.yaml with the HTTP API on a free port and a provider that answers
    When I run foxxycode http with --dry-run --test-config
    Then the command succeeds
    And the report marks httpserver as ok mentioning "free"

  Scenario: foxxycode http reports a port another process holds
    Given a config.yaml with the HTTP API on a port another process holds
    When I run foxxycode http with --dry-run
    Then the command fails
    And the report marks httpserver as an error mentioning "in use"
    And the report points at the line of "port:"
