Feature: A chat can be exported to different document formats
  The editor panel offers a per-session export action once at least one assistant
  answer exists. Exporting renders the user/assistant dialogue (plus reasoning)
  into PDF, DOCX, HTML, or JSON and returns it as a downloadable attachment so
  the transcript can be archived or shared outside the app.

  Background:
    Given a running foxxycode HTTP server
    And a chat with a user question and an assistant answer

  Scenario: The transcript is exported as JSON
    When the panel exports the chat as json
    Then the response is a downloadable JSON attachment
    And the JSON contains the user question and the assistant answer

  Scenario: The transcript is exported as HTML
    When the panel exports the chat as html
    Then the response is a downloadable attachment of type text/html
    And the HTML body contains the assistant answer

  Scenario: The transcript is exported as PDF
    When the panel exports the chat as pdf
    Then the response is a downloadable attachment of type application/pdf
    And the PDF payload begins with the PDF header

  Scenario: The transcript is exported as DOCX
    When the panel exports the chat as docx
    Then the response is a downloadable attachment of type application/vnd.openxmlformats-officedocument.wordprocessingml.document
    And the DOCX payload is a valid Office Open XML package

  Scenario: An unsupported format is rejected
    When the panel exports the chat as rtf
    Then the export request is rejected with status 400

  Scenario: Exporting a missing chat is rejected
    When the panel exports a non-existent chat as json
    Then the export request is rejected with status 404
