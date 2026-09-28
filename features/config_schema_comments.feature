Feature: A saved config keeps its comments and names the editor schema
  Operators edit ~/.foxxycode/config.yaml by hand as often as through the settings screen,
  so the file carries their comments and a "# yaml-language-server:" header pointing a
  YAML language server at FoxxyCode's published JSON Schema. Rewriting the file from the
  settings screen used to serialize the config structs from scratch, which dropped every
  comment - the schema header included, so the editor stopped validating the file right
  after the first save. A save now merges into the file already on disk: comments and key
  order survive, and a config with no header gets the published one. A file an editor on
  Windows wrote is read the same way and written back the same way: the carriage returns
  that used to trail every comment and push a blank line under it on each save are gone,
  and the file keeps the line endings it came with. The skill, subagent and hook locations
  are written back with the ${FOXXYCODE_HOME} and ~ they were read with, so saving an
  unchanged config twice writes the same file, and the file keeps following the agent
  home instead of pinning the directories of the machine that saved it.

  Scenario: A settings save keeps the comments the operator wrote
    Given a foxxycode server whose config.yaml carries operator comments
    When the settings screen saves the config with "agent.max_turns" set to 42
    Then the saved config.yaml still carries the operator comments
    And the saved config.yaml sets "agent.max_turns" to 42

  Scenario: A config an operator wrote on Windows keeps its shape and its line endings
    Given a foxxycode server whose config.yaml carries operator comments in Windows text
    When the settings screen saves the config with "agent.max_turns" set to 42
    Then the saved config.yaml still carries the operator comments
    And the saved config.yaml gained no blank lines
    And the saved config.yaml still ends its lines the Windows way

  Scenario: A saved config points editors at the published schema
    Given a foxxycode server whose config.yaml has no schema header
    When the settings screen saves the config with "agent.max_turns" set to 12
    Then the saved config.yaml starts with the schema header for "https://hijera.github.io/foxxy-agent/config.schema.json"

  Scenario: Settings saves keep the default locations following the agent home
    Given a foxxycode server whose config.yaml leaves skills, subagents and hooks to their default locations
    When the settings screen saves the config with "agent.max_turns" set to 42
    And the settings screen saves the config with "agent.max_turns" set to 42
    Then the saved config.yaml lists "skills.dirs" as "~/.agents/skills, ${FOXXYCODE_HOME}/skills, ${CWD}/.foxxycode/skills"
    And the saved config.yaml lists "subagents.dirs" as "${FOXXYCODE_HOME}/agents, ${CWD}/.claude/agents, ${CWD}/.foxxycode/agents"
    And the saved config.yaml lists "hooks.files" as "${FOXXYCODE_HOME}/hooks.json, ${CWD}/.claude/settings.json, ${CWD}/.claude/settings.local.json, ${CWD}/.foxxycode/hooks.json"
    And both saves wrote the same config.yaml

  Scenario: A settings save keeps the home placeholders the operator wrote
    Given a foxxycode server whose config.yaml lists skill directories under the agent home and the user's home
    When the settings screen saves the config with "agent.max_turns" set to 42
    Then the saved config.yaml lists "skills.dirs" as "~/team-skills, ${FOXXYCODE_HOME}/skills"

  Scenario: A schema header the operator chose is left alone
    Given a foxxycode server whose config.yaml points its editor at "./config.schema.json"
    When the settings screen saves the config with "agent.max_turns" set to 12
    Then the saved config.yaml still points its editor at "./config.schema.json"
    And the saved config.yaml carries exactly one schema header
