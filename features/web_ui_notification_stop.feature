Feature: Completion notifications after Stop
  Scenario: An acknowledged Stop quiets only the cancelled turn
    Then a stopped task stays silent and the next completed task notifies
