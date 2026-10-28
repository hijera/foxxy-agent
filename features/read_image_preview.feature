Feature: The web UI previews a picture the agent looked at
  A read that showed the model a picture keeps it on the call, and the web UI
  shows it under the call's row: a preview card the reader sees without
  opening the row, which opens the original enlarged. The card is there while
  the turn runs and after a reload, and the transcript gains no message the
  operator did not type.

  Scenario: A picture the agent read is previewed on its row
    Then a completed read keeps the picture its call showed the model while the turn streams
    And a reloaded transcript keeps the picture on the read's row
    And the read's row previews the picture without being opened and opens the original enlarged
