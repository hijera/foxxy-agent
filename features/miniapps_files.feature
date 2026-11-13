Feature: Mini Apps carry files through generated forms and persisted results
  Scenario: Upload an input, run a workflow and download its result after reload
    Given a Mini App that reads an uploaded text file and writes a downloadable result
    When I upload a source file for its declared input
    And I test the Mini App with that upload
    Then the successful run exposes a downloadable file containing the source text
    And the file remains downloadable after the Mini Apps service restarts
    And I can verify again without uploading the source a second time
