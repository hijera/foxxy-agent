Feature: Paste-to-chip classification of IDE copies
  Text copied from an IDE editor or terminal and pasted 1:1 into the chat
  composer becomes a mention chip instead of raw text. The IDE reports copy
  candidates to the backend; the composer asks the backend to classify the
  pasted text.

  Background:
    Given a running foxxycode HTTP server

  Scenario: A fragment copied from a file classifies as a line-range mention
    Given the IDE reported copying lines 21-31 of workspace file "Dockerfile" as:
      """
      FROM alpine:3.20
      RUN apk add --no-cache curl
      """
    When the composer classifies the pasted text:
      """
      FROM alpine:3.20
      RUN apk add --no-cache curl
      """
    Then the classification is a file mention of "Dockerfile" lines 21-31

  Scenario: A fragment from a terminal buffer classifies as a terminal mention
    Given the IDE reported terminal "dev" with output:
      """
      npm run dev
      server ready on :3000
      compiled successfully
      """
    When the composer classifies the pasted text:
      """
      server ready on :3000
      compiled successfully
      """
    Then the classification is a terminal mention of "dev"

  Scenario: Text that matches nothing stays a plain paste
    When the composer classifies the pasted text:
      """
      copied from a web page
      somewhere else entirely
      """
    Then the classification is none
