Feature: Updating a FoxxyCode installed from a system package
  A FoxxyCode that apt or dnf put on disk is owned by the package database. Replacing
  that file in place would leave the database describing a build that is no longer
  there, so self-update installs the release package instead - and only a process
  that can write to the package database may do it. Homebrew refuses to run under
  sudo at all, so a brew install has no privileged route and always ends at the
  brew command that upgrades it - and which command that is depends on whether
  brew installed a cask or a formula.

  Scenario: Without root privileges FoxxyCode points at the package manager
    Given FoxxyCode was installed from a deb package
    And FoxxyCode runs without root privileges
    When FoxxyCode tries to install the update
    Then FoxxyCode reports that a package manager owns the installation
    And FoxxyCode names the command that upgrades the package
    And the installed executable is left untouched

  Scenario: Root installs the release package instead of replacing the binary
    Given FoxxyCode was installed from a deb package
    And FoxxyCode runs as root
    When FoxxyCode installs the update
    Then FoxxyCode downloads the release package for this platform
    And FoxxyCode hands the package to the system package manager
    And FoxxyCode reports the release it installed
    And the installed executable is left untouched

  Scenario: A Homebrew cask install is sent back to brew
    Given FoxxyCode was installed by a Homebrew cask
    And FoxxyCode runs as root
    When FoxxyCode tries to install the update
    Then FoxxyCode reports that a package manager owns the installation
    And FoxxyCode tells the user to run "brew upgrade --cask foxxycode"
    And the installed executable is left untouched

  Scenario: A Homebrew formula install is sent back to brew without the cask flag
    Given FoxxyCode was installed by a Homebrew formula
    And FoxxyCode runs as root
    When FoxxyCode tries to install the update
    Then FoxxyCode reports that a package manager owns the installation
    And FoxxyCode tells the user to run "brew upgrade foxxycode"
    And the installed executable is left untouched

  Scenario: Root installs the release package on an rpm system
    Given FoxxyCode was installed from an rpm package
    And FoxxyCode runs as root
    When FoxxyCode installs the update
    Then FoxxyCode downloads the release package for this platform
    And FoxxyCode hands the package to the system package manager
    And FoxxyCode reports the release it installed
