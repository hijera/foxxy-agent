Feature: A chat can be exported to different document formats
  The editor panel offers a per-session export action once at least one assistant
  answer exists. Exporting renders the user/assistant dialogue (plus reasoning)
  into PDF, DOCX, HTML, or JSON and returns it as a downloadable attachment so
  the transcript can be archived or shared outside the app.

  Background:
    Given a running foxxycode HTTP server

  Scenario: The transcript is exported as JSON
    Given a chat with a user question and an assistant answer
    When the panel exports the chat as json
    Then the response is a downloadable JSON attachment
    And the JSON contains the user question and the assistant answer

  Scenario: The transcript is exported as HTML
    Given a chat with a user question and an assistant answer
    When the panel exports the chat as html
    Then the response is a downloadable attachment of type text/html
    And the HTML body contains the assistant answer

  Scenario: The transcript is exported as PDF
    Given a chat with a user question and an assistant answer
    When the panel exports the chat as pdf
    Then the response is a downloadable attachment of type application/pdf
    And the PDF payload begins with the PDF header

  Scenario: The transcript is exported as DOCX
    Given a chat with a user question and an assistant answer
    When the panel exports the chat as docx
    Then the response is a downloadable attachment of type application/vnd.openxmlformats-officedocument.wordprocessingml.document
    And the DOCX payload is a valid Office Open XML package

  Scenario: An unsupported format is rejected
    Given a chat with a user question and an assistant answer
    When the panel exports the chat as rtf
    Then the export request is rejected with status 400

  Scenario: Exporting a missing chat is rejected
    Given a chat with a user question and an assistant answer
    When the panel exports a non-existent chat as json
    Then the export request is rejected with status 404

  Scenario: A chat the panel would not offer to export is refused
    Given a chat with only a user question
    When the panel exports the chat as json
    Then the export request is rejected with status 404

  Scenario: Emphasis inside a sentence does not break the PDF paragraph apart
    Given a chat whose answer mixes bold and inline code inside one sentence
    When the panel exports the chat as pdf
    Then the sentence is laid out on a single line of the PDF
    And consecutive paragraphs are separated by vertical space

  Scenario: A pasted terminal snippet still yields a document Word can open
    Given a chat whose question carries terminal escape codes
    When the panel exports the chat as docx
    Then the DOCX document part is well-formed XML
    And the surrounding text of the snippet survives

  Scenario: The DOCX only uses paragraph styles it defines
    Given a chat whose answer uses headings from level one to level six
    When the panel exports the chat as docx
    Then every paragraph style used by the document is defined in the style sheet

  Scenario: List items carry their marker exactly once
    Given a chat whose answer contains a bullet list and a numbered list
    When the panel exports the chat as docx
    Then no list item repeats the marker glyph in its own text
    And the numbered list is numbered by the document rather than bulleted

  Scenario: A non-ASCII chat title survives the download
    Given a chat titled "Отчёт по задаче" with an assistant answer
    When the panel exports the chat as pdf
    Then the attachment offers the UTF-8 filename "Отчёт_по_задаче.pdf"
    And the attachment keeps an ASCII filename fallback

  # The agent appends the editor's ambient state to every user turn. Nobody typed
  # it, so a document meant to be read drops it; the JSON export keeps it, being
  # the machine-readable one.
  Scenario Outline: A readable export drops the ambient IDE and terminal context
    Given a chat whose question carries injected IDE and terminal context
    When the panel exports the chat as <format>
    Then the document still carries the question the user typed
    And the document shows no active file, open tabs or terminal section

    Examples:
      | format |
      | html   |
      | pdf    |
      | docx   |

  Scenario: The JSON export keeps the injected context
    Given a chat whose question carries injected IDE and terminal context
    When the panel exports the chat as json
    Then the document still carries the question the user typed
    And the JSON still carries the injected context blocks

  # An editor webview cannot save a blob, so the panel asks the server to write
  # the document out and lets the plugin reveal it in the OS file manager.
  Scenario: An editor panel receives the export as a file on disk
    Given a chat titled "Отчёт по задаче" with an assistant answer
    When the editor panel exports the chat to a file as pdf
    Then the response carries the absolute path of a readable pdf file
    And the file is named after the chat title

  Scenario: Revealing the exported file is offered to the connected IDE
    Given a chat with a user question and an assistant answer
    And an editor plugin listening for IDE events
    When the editor panel exports the chat to a file as docx
    Then the IDE is asked to reveal the exported file

  Scenario: Re-exporting the same chat replaces the file instead of piling up copies
    Given a chat with a user question and an assistant answer
    When the editor panel exports the chat to a file as json
    And the editor panel exports the chat to a file as json
    Then the export directory holds exactly one json file

  # Windows locks a .docx that is still open in Word, so overwriting the previous
  # export fails. Exporting again should still hand the user a document.
  Scenario: Exporting again while the previous document is open picks a new name
    Given a chat with a user question and an assistant answer
    And the previously exported document cannot be replaced
    When the editor panel exports the chat to a file as docx
    Then the response carries the absolute path of a readable docx file
    And the file name carries a numeric suffix

  Scenario: The file route refuses a chat the panel would not offer to export
    Given a chat with only a user question
    When the editor panel exports the chat to a file as json
    Then the export request is rejected with status 404
