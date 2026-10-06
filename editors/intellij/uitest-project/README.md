# UI test fixture project

The sandbox IDE started by `runIdeForUiTests` opens a copy of this directory, because the
FoxxyCode tool window is a *project* tool window — without an open project the IDE stops at the
Welcome frame and there is nothing to automate.

It is copied to `build/uitest-project/` before the IDE launches, so the `.idea/` directory the IDE
creates stays out of the repository. Keep it tiny: every file here is indexed on every launch.
