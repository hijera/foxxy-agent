Feature: Configuration migration preserves private settings
  Scenario: Load legacy declarations and a merged unlimited step budget
    Given a private legacy configuration with an environment skill source and a merged zero step limit
    When the configuration is loaded
    Then the migrated declarations are private and keep their values
    And the agent step limit remains unlimited
