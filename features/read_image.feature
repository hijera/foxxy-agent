Feature: The model looks at an image file with read
  read hands the model the picture in a PNG, JPEG, GIF or WebP file when the
  session's model reads images (models[].multimodal). The picture stays with
  the result of the read that produced it, so the transcript gains no message
  nobody typed and every surface finds the picture on that call: the web UI
  previews it on the read row, a Telegram chat receives it as a photo, the
  console and an editor show the read's text. What the provider is sent
  carries the pictures of one step in a single user message right after the
  step's tool results, because an OpenAI-compatible tool result cannot hold
  an image and nothing may come between the results of one step.

  Scenario: Two images read in one step reach the model after the tool results
    Given a model that reads images
    And workspace images "before.png" and "after.png"
    When the model reads "before.png" and "after.png" in one step, then answers
    Then the next LLM request has the assistant tool calls, then both tool results, then one user message
    And that user message carries the images "before.png" and "after.png" in that order
    And the persisted transcript keeps each image on the result of the read that produced it
    And the persisted transcript has no user message besides the prompt
    And each read tells the surfaces about its image, saved with the session's assets
