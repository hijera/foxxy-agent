Feature: Inline marketplace skill paths scope a plugin in a shared repository
  A marketplace may point several plugins at the same repository root and
  give each one its own skills paths, as anthropics/skills does.

  Scenario: Installing and updating one plugin leaves its neighbours uninstalled
    Given an added marketplace whose plugins share a source root and declare separate skills
    When I run the scoped plugin command "install one@scoped"
    Then only the selected skill "one" is installed
    When I run the scoped plugin command "marketplace update scoped"
    Then only the selected skill "one" is installed
