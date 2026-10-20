Feature: foxxycode serve runs as a systemd user service
  On Linux a foxxycode serve that has to outlive the terminal, come back after a crash
  and start again after a reboot belongs to the user's systemd manager. How the
  unit gets there depends on how FoxxyCode was installed. The .deb and .rpm put
  foxxycode.service under /usr/lib/systemd/user and enable nothing, because installing
  a package must not start a server for every account on the machine. The install
  script puts a binary under the user's home and no unit at all. Either way the
  user runs `foxxycode serve install` once, as themselves: it checks the configuration,
  writes a unit for a binary that came without one, enables and starts the
  service, and says where its log is. `foxxycode serve uninstall` takes the service
  away again and leaves the configuration, the sessions and the workspace alone.

  The service works in ~/FoxxyCode. That folder is the workspace of every session
  opened without one, so what the agent writes lands in a folder of its own and
  not next to the configuration and the provider keys in ~/.foxxycode. And it finds
  the tools a terminal finds: a user manager starts services with a bare system
  PATH, so install hands the service the PATH of the shell it ran from.

  Scenario: install writes and starts the unit for a binary the install script put in place
    Given a Linux account whose foxxycode runs from "~/.local/bin/foxxycode"
    And the account has a valid configuration
    When the user runs foxxycode serve install
    Then the unit "~/.config/systemd/user/foxxycode.service" runs "~/.local/bin/foxxycode serve"
    And the unit works in "~/FoxxyCode"
    And the folder "~/FoxxyCode" exists
    And the service gets the PATH of the shell install ran from
    And systemd reloaded its units, enabled foxxycode.service and restarted it
    And install reports the service running and how to read its log

  Scenario: install enables the unit the package installed
    Given a Linux account whose foxxycode is the packaged binary with its unit
    And the account has a valid configuration
    When the user runs foxxycode serve install
    Then no unit is written under "~/.config/systemd/user"
    And the folder "~/FoxxyCode" exists
    And the service gets the PATH of the shell install ran from
    And systemd reloaded its units, enabled foxxycode.service and restarted it
    And install reports the service running and how to read its log

  Scenario: uninstall removes the service install wrote and keeps the user's files
    Given a Linux account whose foxxycode runs from "~/.local/bin/foxxycode"
    And the account has a valid configuration
    And the user ran foxxycode serve install
    When the user runs foxxycode serve uninstall
    Then systemd disabled and stopped foxxycode.service
    And the unit "~/.config/systemd/user/foxxycode.service" is gone
    And the PATH install handed to the service is gone
    And the configuration and the folder "~/FoxxyCode" are still there

  Scenario: uninstall leaves the packaged unit to the package
    Given a Linux account whose foxxycode is the packaged binary with its unit
    And the account has a valid configuration
    And the user ran foxxycode serve install
    When the user runs foxxycode serve uninstall
    Then systemd disabled and stopped foxxycode.service
    And the packaged unit is still installed

  Scenario: the service is restarted when a configuration change moves a listen address
    Given foxxycode serve was started by the unit install writes
    Then it may end itself for a restart
    And the unit starts it again on the status it exits with
