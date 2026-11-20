Feature: Restricted workspace searches keep the agent home private
  A messenger user who is not an admin may search the workspace, but must
  not read the agent's credentials when its home is inside that workspace.
  Every search tool the turn may call - grep, glob and print_tree - keeps
  the home out of its results, and none of its entries uses up a result
  limit that belongs to the workspace.

  Scenario Outline: A recursive search skips the nested agent home
    Given an agent home inside the session workspace
    When a restricted turn searches for "<pattern>" with path "<path>" and limit <limit>
    Then the search returns the workspace note without the agent credential

    Examples:
      | pattern | path | limit |
      | needle  | .    | 10    |
      | needle  |      | 10    |
      | needle  | .    | 1     |
      | секрет  | .    | 10    |
      | секрет  |      | 10    |
      | секрет  | .    | 1     |

  Scenario: A file listing skips the nested agent home
    Given an agent home inside the session workspace
    When a restricted turn lists files matching "**/*"
    Then the listing shows the workspace note without any agent-home path

  Scenario: A directory tree skips the nested agent home
    Given an agent home inside the session workspace
    When a restricted turn prints the directory tree
    Then the tree shows the workspace note without any agent-home path
