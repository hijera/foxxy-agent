Feature: Select a NeuralDeep deployment before entering credentials
  The onboarding form offers the same official domains as Settings.
  The chosen domain determines the hub, model catalog and saved provider.

  Scenario: Choose a domain before using a manual key
    Then onboarding offers the NeuralDeep domain before credentials and uses it for models and save

  Scenario: Sign in through the chosen domain
    Then onboarding uses the selected NeuralDeep domain for hub sign in
