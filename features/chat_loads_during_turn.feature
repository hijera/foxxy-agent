Feature: A chat opens while its turn is still running
  Reloading the editor panel in the middle of a long answer must bring the conversation
  back. Reading one chat and listing the chats both have to work while an agent turn is
  writing to that same session, and neither may be held up by the other.

  Background:
    Given a running foxxycode HTTP server
    And a long agent turn in flight for my chat

  Scenario: The transcript is served while the turn is still writing
    When the panel opens the chat
    Then the chat loads with the question it started from
    And the chat is listed in history as still working

  Scenario: Opening the chat does not wait for the history list
    Given a workspace with many long chats
    When the panel opens the chat
    Then the chat loads with the question it started from
    And listing the chats stays responsive
