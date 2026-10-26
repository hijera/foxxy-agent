Feature: FoxxyCode on Android under Termux
  The Android release is the Go binary built with GOOS=android for arm64 and
  x86_64, linked by the NDK against Bionic: a position independent executable
  that names Android's system linker as its interpreter. Termux runs it
  directly where Android lets the app execute the files in its data directory,
  and as `/system/bin/linker64 <path>` where it does not, which is every device
  running a Termux that targets Android 10 or later, such as the Google Play
  build. FoxxyCode then does for the programs it starts what termux-exec does for
  Termux's own: it runs them through the same linker, and it finds the
  interpreters that scripts name by their Linux paths in the Termux prefix.

  Scenario: Knowing its own binary when the system linker started FoxxyCode
    Given the system linker started FoxxyCode with the arguments "serve --daemon"
    When FoxxyCode looks for its own binary
    Then it finds the binary the linker was given rather than the linker
    And its arguments are still "serve --daemon"

  Scenario: Running a shell command when Termux starts programs through the linker
    Given the system linker started FoxxyCode
    And bash is installed in the Termux prefix
    When FoxxyCode starts bash for a command
    Then the command runs the system linker on bash
    And bash learns its own path from TERMUX_EXEC__PROC_SELF_EXE

  Scenario: Running a Node script whose shebang names /usr/bin/env
    Given the system linker started FoxxyCode
    And npx is installed in the Termux prefix with the shebang "#!/usr/bin/env node"
    When FoxxyCode starts npx for an MCP server
    Then the command runs the system linker on the env of the Termux prefix
    And env receives node and the path of the script

  Scenario: Running the same script where Termux executes its files directly
    Given the kernel started FoxxyCode directly
    And npx is installed in the Termux prefix with the shebang "#!/usr/bin/env node"
    When FoxxyCode starts npx for an MCP server
    Then the command runs the env of the Termux prefix directly
    And env receives node and the path of the script
