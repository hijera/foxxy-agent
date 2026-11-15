Feature: Formula rendering preserves code in list items
  Code examples remain literal when a fence starts on the same line as a list marker.

  Scenario: Copy a Python example containing LaTeX delimiters
    Then a code fence in a list item displays and copies its literal LaTeX delimiters
