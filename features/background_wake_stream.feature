Feature: A woken turn is watchable while it runs
  A turn started by a finished background task is the one turn nobody asked for over HTTP,
  so it used to produce no stream at all: a chat open on that session found the session busy
  with nothing to attach to, sat on a status line built from the transcript as it was before
  the wake, and only learned what happened on reload. A woken turn therefore publishes the
  same composer stream a user turn does. Waiting for a busy session must stay harmless: the
  wake takes the stream only once the turn is actually its own.

  Scenario: A watching chat receives the woken turn live
    Given a running foxxycode http server with a session
    When a finished background task wakes that session
    And I subscribe to the composer stream of that session
    Then the stream carries the woken turn's answer
    And the stream ends with [DONE]

  Scenario: A wake waiting for a busy session leaves the running turn's stream alone
    Given a running foxxycode http server with a session
    And a user turn is streaming on that session
    When a finished background task wakes that session
    Then the user turn keeps its own composer stream
