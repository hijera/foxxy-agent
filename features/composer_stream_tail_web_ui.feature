Feature: A composer stream ending on its terminal marker counts as complete
  An SSE frame ends with a blank line, and a composer turn ends with the terminal
  `data: [DONE]` frame. A proxy or the embedded browser can hand the client that
  terminal frame without the blank line that closes it, so it is still sitting in
  the reader's trailing buffer when the stream closes. The client must count such
  a stream as complete: a finished answer is not put into recovery, and the panel
  does not replace it with an older copy from disk. A stream that closes before
  its terminal frame - a real cut mid-turn - still counts as cut.

  Scenario: The terminal marker in the unterminated tail completes the stream
    Then a [DONE] frame delivered without the closing blank line ends the stream as complete

  Scenario: A stream cut before the terminal marker stays incomplete
    Then a reader that closes before [DONE] reports the stream as cut mid-turn
