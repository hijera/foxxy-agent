Feature: Onboarding status follows the agent's own provider
  The embedded UI asks GET /foxxycode/onboarding/status once per page load and
  opens the provider picker when the agent cannot run a turn. A second provider
  row that was saved without a key - typically an "openai" row retyped to
  neuraldeep before its hub login exists - is listed in missing_api_keys for
  information, but must not reopen the picker after every Settings save and on
  every start while the agent's own provider is keyed.

  @http
  Scenario: Retyping a keyless openai provider to neuraldeep saves without reopening onboarding
    Given a foxxycode HTTP server whose config has a keyed neuraldeep agent provider and a keyless openai provider
    When I retype the openai provider to neuraldeep through the settings REST flow
    Then the save succeeds and the saved config holds the openai provider as neuraldeep
    And the onboarding status keeps the provider picker closed
    And the onboarding status lists "openai" among the providers without a key

  @http
  Scenario: The hub sign-in widget answers for the retyped row before it is saved
    Given a foxxycode HTTP server whose config has a keyed neuraldeep agent provider and a keyless openai provider
    When I ask the NeuralDeep sign-in status for the openai provider before saving
    Then the sign-in status reports the openai provider disconnected instead of a conflict

  @http
  Scenario: A key from the conventional environment variable counts
    Given OPENAI_API_KEY is set in the environment
    And a foxxycode HTTP server whose config has a keyed neuraldeep agent provider and a keyless openai provider
    When I retype the openai provider to neuraldeep through the settings REST flow
    Then the save succeeds and the saved config holds the openai provider as neuraldeep
    And the onboarding status lists no providers without a key

  @http
  Scenario: The picker opens when the agent's own provider has no credentials
    Given the agent model is going to point at the openai provider
    And a foxxycode HTTP server whose config has a keyed neuraldeep agent provider and a keyless openai provider
    When I retype the openai provider to neuraldeep through the settings REST flow
    Then the save succeeds and the saved config holds the openai provider as neuraldeep
    And the onboarding status asks for the provider picker
