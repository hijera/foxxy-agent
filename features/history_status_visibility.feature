Feature: Saved History status filters remain visible
  A saved archive filter must not make existing chats look lost after a reload.

  Scenario: Return from an empty archive to active chats
    Then an empty archived History names its selected status and offers active chats

  Scenario: Restart an IDE that chooses a new backend port
    Then the project-only choice survives a fresh localStorage origin
    And collapsed groups survive a new IDE port without mixing environments
