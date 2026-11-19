Feature: Linked Mermaid nodes in rendered pictures
  A Mermaid node with a URL remains a valid standalone SVG picture in the editor panel.

  Scenario: A linked node keeps its SVG namespace
    Then a linked Mermaid node renders with a valid xlink namespace
