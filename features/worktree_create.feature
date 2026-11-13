Feature: Create a feature worktree during an agent turn
  The session follows the worktree so subsequent tools run in it.

  Scenario: The next shell command uses the new worktree without a cwd argument
    Given a session in a repository with a bare origin
    When the agent creates a worktree for "feature/login"
    And the agent runs "git rev-parse --show-toplevel" without a cwd argument
    Then the command runs inside the new worktree

  Scenario: The default branch is refused and the session does not move
    Given a session in a repository with a bare origin
    When the agent tries to create a worktree for "main"
    Then the creation is refused and the session stays in the main checkout
