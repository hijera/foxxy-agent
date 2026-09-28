Feature: A saved config keeps the process-scoped locations as written
  sessions.dir, memory.dir, logger.file and scheduler.dir belong to the process, so the
  loader resolves them to absolute paths when it reads config.yaml: ${FOXXYCODE_HOME},
  ${CWD}, environment references and a leading ~ are expanded, and an unset scheduler.dir
  becomes <home>/scheduler. Saving from the settings screen used to write those absolute
  paths into the file, so the first save pinned it to the machine and the user it was
  saved on: the file stopped following FOXXYCODE_HOME (--home), and a copy on another
  machine or on a Docker volume pointed at the old home. A save now writes each of them
  back the way the file had it, for as long as the value is unchanged.

  Scenario: Saving an unchanged config leaves the default scheduler directory unset
    Given a foxxycode server whose config.yaml leaves scheduler.dir unset
    When the settings screen saves the config unchanged
    And the settings screen saves the config unchanged again
    Then the saved config.yaml leaves "scheduler.dir" empty
    And the saved config.yaml names no path under the agent home
    And both saves wrote the same config.yaml

  Scenario: Saving an unchanged config keeps the placeholders the operator wrote
    Given a foxxycode server whose config.yaml spells its locations with placeholders
    When the settings screen saves the config unchanged
    And the settings screen saves the config unchanged again
    Then the saved config.yaml sets "sessions.dir" to "${FOXXYCODE_HOME}/team-sessions"
    And the saved config.yaml sets "memory.dir" to "~/foxxy-memory"
    And the saved config.yaml sets "logger.file" to "${FOXXYCODE_HOME}/logs/agent.log"
    And the saved config.yaml sets "scheduler.dir" to "${CWD}/jobs"
    And the saved config.yaml names no path under the agent home
    And both saves wrote the same config.yaml

  Scenario: A saved config follows the agent home it is read with
    Given a foxxycode server whose config.yaml leaves scheduler.dir unset
    When the settings screen saves the config unchanged
    Then the saved config.yaml, read with another agent home, keeps "scheduler.dir" under that home
