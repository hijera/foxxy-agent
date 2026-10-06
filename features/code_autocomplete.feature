Feature: Inline code autocomplete
  With autocomplete enabled, an editor plugin asks the backend for the text to
  insert at the caret and renders it as a greyed suggestion. The request carries
  the code on both sides of the caret, so the model completes the middle rather
  than continuing the file, and the reply is cleaned into text the editor can
  insert verbatim. Editors read their behaviour (enabled, trigger, debounce)
  from the same section, so the knobs live only in config.autocomplete.

  Scenario: An editor asks for a suggestion at the caret
    Given a running foxxycode HTTP server with autocomplete enabled
    When an editor requests a completion for a caret inside a function
    Then the suggestion comes back as text that can be inserted verbatim
    And the model was given the code on both sides of the caret

  Scenario: An editor learns how to behave before it starts asking
    Given a running foxxycode HTTP server with autocomplete enabled
    When an editor reads the autocomplete client config
    Then the client config reports autocomplete enabled with a trigger and a debounce
