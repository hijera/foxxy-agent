Feature: Restricted workspace searches keep the agent home private
  A messenger user who is not an admin may search the workspace, but must
  not read the agent's credentials when its home is inside that workspace.

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
