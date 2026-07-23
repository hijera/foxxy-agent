Feature: Design plan card in a plan-mode turn
  A plan-mode turn writes a design plan mid-turn and keeps talking afterwards.
  The transcript row and the design-plan SSE event are what the bundled UI uses to
  render the plan card as the last row of the turn, with Discard and Run plan.

  Scenario: A plan written mid-turn is published to the client and kept in the transcript
    Given a running foxxycode HTTP server with a planning agent
    And an HTTP session in plan mode
    When the user asks for a plan
    Then the turn streams a design plan event carrying the plan slug
    And the session transcript contains a plan document row for that slug
    And the assistant text of that turn is persisted after the plan was written
    And the plan card shows the plan name from the frontmatter

  Scenario: A plan whose frontmatter lost its --- fences still yields a usable card
    Given a running foxxycode HTTP server with a planning agent that omits frontmatter fences
    And an HTTP session in plan mode
    When the user asks for a plan
    Then the plan card shows the plan name from the frontmatter
    And the plan card body holds markdown rather than raw frontmatter
    And the plan file on disk starts with a frontmatter fence

  Scenario: The model may finish planning and start the implementation itself
    Given a running foxxycode HTTP server with a planning agent that leaves plan mode
    And an HTTP session in plan mode
    When the user asks for a plan
    Then the session mode is "agent"

  Scenario: Forbidding self-run keeps the session in plan mode
    Given a running foxxycode HTTP server with a planning agent that leaves plan mode
    And the model is forbidden from running the plan itself
    And an HTTP session in plan mode
    When the user asks for a plan
    Then the session mode is "plan"
    And the refused tool is reported back to the model
