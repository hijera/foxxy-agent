# IntelliJ and Android acceptance fixes

Actual IntelliJ IDEA Community 2022.2.1 / JCEF, Windows x64; Android 14 x86_64 under Termux UID, connected through loopback adb forward/reverse. Isolated test homes and a local deterministic model stand; the image is the public repository fixture.

- Before: Android requests work but the old JCEF lacks AbortSignal.timeout and displays a false remote-down banner.
- After: no false banner; the environment menu reports the Android server up in the same JCEF runtime.
- Before: selecting claude-api from anthropics/skills installed 19 unrelated skills and failed to install the selected one.
- After: one skill added, zero failures; update touches only that skill and preserves its disabled state.
- Windows and Android HTML/PDF/DOCX exports were inspected: each embeds one picture from the read tool's saved session asset.
- The permanent IntelliJ suite passes seven tests, including no new TransactionGuard errors when switching light/dark themes.

Before captures are 700 px; after captures are 1280 px. These are real JCEF viewport captures, not desktop screenshots of unrelated windows. The PR records the final full-suite, lint, native and cross-build results.
