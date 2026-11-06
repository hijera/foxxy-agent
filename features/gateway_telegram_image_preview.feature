Feature: A Telegram chat sees the pictures the agent looked at
  When the agent of a chat reads an image file and its model reads images, the
  bot sends that picture into the chat as a photo captioned with the file's
  name while the turn runs, so the person sees what the agent looked at before
  the answer comes. The photo is the copy FoxxyCode kept with the session, the
  picture the model was shown.

  Scenario: A picture the agent read reaches the chat as a photo
    Given a chat whose agent reads images and a screenshot "shot.png" in the workspace
    When the person asks the agent to look at the screenshot
    Then the chat receives "shot.png" as a photo of the screenshot
    And the answer "It is a red square." follows in the chat
