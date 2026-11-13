Feature: Reveal a shared source on the Windows desktop
  A desktop reveal must show the containing folder, even when the editor's
  backend has no console. Starting a process alone is not success.

  Scenario: Reveal a source with spaces, Unicode and a comma in its path
    Given a Windows desktop source file with spaces Unicode and a comma
    When a backend without a console reveals the source
    Then Explorer visibly opens the source folder
