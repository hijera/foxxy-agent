Feature: Separate step and turn clocks in the live status
  Scenario: A long command keeps its clock distinct from the turn totals
    Then the command clock and turn totals stay separated at phone and desktop widths in both locales and themes
