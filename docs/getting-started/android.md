# Android (Termux)

FoxxyCode runs on Android inside [Termux](https://termux.dev) from a release built for Android:
**`foxxycode_X.Y.Z_android_arm64.tar.gz`** for 64-bit ARM (aarch64), the processor of phones and
tablets, and **`foxxycode_X.Y.Z_android_amd64.tar.gz`** for x86-64 (x86_64), which Android runs on in
emulators, on Chromebooks and on PCs. It is the whole FoxxyCode: the console, **`foxxycode serve`** with the
web UI, the Telegram bot and **`foxxycode acp`**. Android 7.0 or later is needed, as for Termux itself.

## Install

Download the Android archive from [FoxxyCode releases](https://github.com/hijera/foxxy-agent/releases). Choose **arm64** for most phones, or **amd64** for x86-64. Replace `X.Y.Z` below with the release version:

```bash
pkg install curl
curl -fLO https://github.com/hijera/foxxy-agent/releases/download/X.Y.Z/foxxycode_X.Y.Z_android_arm64.tar.gz
tar -xzf foxxycode_X.Y.Z_android_arm64.tar.gz
mkdir -p ~/.local/bin
install -m 755 foxxycode ~/.local/bin/foxxycode
export PATH="$HOME/.local/bin:$PATH"
foxxycode --help
```

Add the `PATH` line to `~/.bashrc` to retain it. The archive also contains the man page and shell completions; install them as described in [Install](install.md#release-archive). The upstream `coddy.dev` installer installs coddy-agent, so use this fork's release archives.

## Why the Linux archive does not run

The Linux archives hold a static Linux executable, and Termux cannot start it everywhere.
Android 10 and later refuse an app that targets them the right to execute files from its own data
directory, so a Termux that targets them, such as the Google Play build, starts every program
through Android's linker, **`/system/bin/linker64 <program>`**. The linker loads position-independent
executables only and turns the static one away:

```text
error: "/data/data/com.termux/files/home/.local/bin/foxxycode" has unexpected e_type: 2
```

Where Termux executes files directly (the F-Droid and GitHub builds), the Linux binary starts, but
the Linux paths it relies on are missing: it finds no CA certificates and no **`/etc/resolv.conf`**,
so every request to a provider fails, and on the Android versions whose seccomp filter rejects the
**`faccessat2`** system call it is killed with **`Bad system call`** the first time it looks a
program up.

The Android archives are built with **`GOOS=android`** and linked by the Android NDK against
Bionic, Android's C library: a position-independent executable that names **`/system/bin/linker64`**
as its interpreter and needs only libraries every Android carries (**`libc`**, **`libdl`**,
**`liblog`**), which both kinds of Termux start. Like any Termux program, it resolves hostnames
through Android itself, so the network's DNS, Private DNS and a VPN's DNS all apply. The rest of
this page is what the build does differently at run time.

## What FoxxyCode adapts on Android

Nothing needs configuring: FoxxyCode reads the Termux environment when it starts.

- **Programs it starts.** **`run_command`**, background tasks, hooks, MCP servers over stdio, git and
  ripgrep are started the way Termux starts its own programs. Where Termux goes through
  **`/system/bin/linker64`**, FoxxyCode does too, and a script is handed to its interpreter; a script
  whose first line names **`/usr/bin/env`** or **`/bin/sh`** gets the Termux program of that name.
  This is what the **`termux-exec`** library does for Termux's own programs; FoxxyCode, like any Go
  program, starts processes with a system call the library cannot see, so it does the same itself.
  A shell command runs in Termux's **`bash`** and behaves as it does in your session.
- **Its own path.** Started through the linker, FoxxyCode finds its binary from the command line rather
  than **`/proc/self/exe`**, which names the linker, so **`foxxycode update`** and
  **`foxxycode serve --daemon`** reach the right file.
- **Certificates.** FoxxyCode trusts the bundle of Termux's **`ca-certificates`** package,
  **`$PREFIX/etc/tls/cert.pem`**, next to Android's system store. It does so through
  **`SSL_CERT_FILE`**, so a bundle you export yourself wins.
- **Temporary files.** **`TMPDIR`** falls back to **`$PREFIX/tmp`**: the Android default,
  **`/data/local/tmp`**, is not writable by an app.
- **Opening a browser.** Sign-in flows such as **`foxxycode providers login`** open the phone's browser
  with **`termux-open-url`**, and the browser reaches the callback FoxxyCode listens for on the device's
  loopback address.
- **Updates.** **`foxxycode update`** downloads the Android archive of the new release and replaces the
  binary in place; see [Update](update.md).

## Running `foxxycode serve`

**`foxxycode serve`** works as elsewhere, and the web UI opens at **`http://127.0.0.1:12345/`** in the
phone's browser. Android has no systemd, so **`foxxycode serve install`** is not available: run the
server in a Termux session, in **`tmux`** (**`pkg install tmux`**), or in the background with
**`foxxycode serve --daemon`** (see [foxxycode serve](../operate/serve.md)).

Android stops the processes of an app it considers idle. While the server has to stay up, take
Termux's wake lock (**`termux-wake-lock`**, or from the Termux notification) and let Termux run
unrestricted in the battery settings. Android 12 and later also kill the background processes of an
app beyond a limit (the "phantom process killer"); Termux's documentation describes how to lift it.

A phone can also be a node of a swarm: it dials a relay on a laptop or a server, and the relay's
web UI and the console drive it like any other machine, with no open port on the phone. The whole
path is in [Android phones as swarm nodes](../tutorials/swarm-android-nodes.md).

## Limitations

- Only the 64-bit builds are published, arm64 and x86_64: a 32-bit ARM or x86 Android device is
  not covered.
- A program that is itself a static executable cannot run where Termux starts programs through the
  linker, whoever starts it; that is Android's rule, and the error is the same **`unexpected e_type`**.

## Building it yourself

**`make android`** cross-compiles **`build/foxxycode-android-arm64`** and **`build/foxxycode-android-amd64`**
with **`TAGS`** as for **`make build`**, and **`make check-android`** type-checks the Android build.
Both need the Android NDK, which links the binaries against Bionic; see
[Build from source](../contributing/build.md#android-termux).
