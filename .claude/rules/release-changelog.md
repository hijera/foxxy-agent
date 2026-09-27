---
description: Post-merge version and the user-facing changelogs (IntelliJ + VS Code) before opening a PR
paths:
  - "editors/**"
  - "**/CHANGELOG.md"
  - "**/CHANGELOG.en.md"
---

# Release version and changelogs (before every PR)

Merging a PR into `main` releases immediately: `.github/workflows/tag-on-merge.yaml` tags the
merge commit and the plugin/binary workflows publish from that tag. There is no separate
release step where someone writes release notes afterwards — so the notes are part of the PR.

There are **two** changelogs, one per editor plugin, and both follow the same rules. Each has an
**English twin** next to it, `CHANGELOG.en.md`, read by the project website (see §4):

| File | Shown to the user as | Stamped by |
| --- | --- | --- |
| `editors/intellij/CHANGELOG.md` | **Change Notes** tab in `Settings \| Plugins` | `build.gradle.kts` (parses the file into `plugin.xml`) |
| `editors/vscode/CHANGELOG.md` | **Changelog** tab on the extension page in VS Code | `make vscode-package` → `scripts/stamp-changelog.mjs` (rewrites the heading in the packaged copy) |

Both plugins release under the **same tag** (one patch bump per merge), so a version may have a
section in one file, both, or neither — a section appears only where that plugin's user can
see the change.

## 1. Do not guess the version — write `Unreleased`

CI takes the **highest SemVer tag on `origin`** and bumps the **patch**, so the version a PR
releases under depends on **what merges before it**. While your PR is open that number is not
knowable: if another PR merges first, yours moves up by one and any number you wrote down is
already wrong.

The minor version moves only when `MIN_VERSION` in `tag-on-merge.yaml` — the floor of the current
release line, `0.3.0` for the `0.3.x` section of `ROADMAP.md` — is raised: the first merge after
that releases the floor itself instead of a patch bump. One more reason the number is not yours
to write.

So do not write one. The newest section is headed:

```markdown
## Unreleased — 2026-09-04
```

The build replaces `Unreleased` with the version it is actually building — the tag the release
workflow just created — so the heading users see always matches the plugin version they
installed. Only the date is yours to write. (`build.gradle.kts` does it for IntelliJ;
`make vscode-package` / `vscode-package-target` snapshot `CHANGELOG.md` to
`CHANGELOG.md.vsce.bak`, run `scripts/stamp-changelog.mjs <version>`, package, and restore the
file, so the source tree stays clean. Dev builds — `0.0.0-dev-<sha>` — keep the `Unreleased`
heading on purpose.)

> This replaces the old "predict the next tag" rule, which produced exactly the failure it
> invited: four sections in `editors/intellij/CHANGELOG.md` ended up headed one version above
> the tag that shipped them, because another PR merged between the guess and the merge. Those
> four were corrected against the tags; the guessing is what got removed.

**When you add your section**, the `## Unreleased` already at the top (if any) belongs to a
release that has since been tagged. Give it its real number first — this is a lookup, not a
guess, because that release exists now:

```bash
git ls-remote --tags origin | grep -oE '[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1
```

That is the tag the previous `## Unreleased` shipped under. Rename its heading to
`## X.Y.Z — <its own date>`, then add your own `## Unreleased — <today>` above it. Do this in
**each** changelog that has an `Unreleased` section, even one you are not otherwise touching, and
in its English twin: the previous author's notes shipped under that tag in both plugins. If nobody does this the
notes still ship correctly — the build stamps whatever is on top — the file just keeps coarser
history, so it is worth doing.

`editors/intellij/changelog_test.go` (`make test`) and `editors/vscode/test/changelog.test.ts`
(`npm test` in `editors/vscode`, also run by the VS Code CI job) enforce the shape: at most one
`## Unreleased`, and it must be the newest section.

## 2. Write the entry

Section format, newest first — the top one unnumbered, the released ones numbered:

```markdown
## Unreleased — 2026-09-04

**Короткий заголовок изменения.**
Что именно изменилось для того, кто пользуется плагином, и почему это заметно.

## 0.2.23 — 2026-08-03
...
```

Rules for the text, enforced by both test suites above:

- **In Russian.** These files are read by users, not by CI. An entry with no Cyrillic fails.
- **In human terms, not commits.** Describe the behaviour that changed and what the user
  will notice. Not "port upstream d00bcd7", not a bullet list of commit subjects.
- **Not a stub.** `TBD` or an empty section fails.
- **Versions strictly descending**, one section per released version.
- **No merge-conflict markers.** A conflicted changelog still parses and still reads as
  Russian, so nothing else catches it — and 0.2.46 shipped with a `=======` paragraph in the
  Change Notes tab. Resolving a changelog after a merge means picking both sides' sections,
  in descending order, not leaving the markers in.
- **No authoring notes in the VS Code file.** IntelliJ renders only the `##` sections, so
  `editors/intellij/CHANGELOG.md` can carry an explanatory preamble; VS Code renders
  `editors/vscode/CHANGELOG.md` **verbatim** in the Changelog tab, so nothing but the `# title`
  line and the sections may be visible there. Notes for authors go in an HTML comment
  (`<!-- … -->`) or in this rule; `changelog.test.ts` fails on any other preamble text.

Everything else in the repository — commit messages, `UPSTREAM_SYNC.md`, `docs/` — stays in
English per `AGENTS.md`. These two files are the deliberate exception, because they are
product copy shown inside the editor. Their English twins are covered in §4.

## 3. Which changelog gets the entry

Write to every plugin whose **user can observe** the change, in that plugin's own words
(the same feature can surface differently: a Settings-page button in IntelliJ, a Command
Palette entry in VS Code):

- **Both** — anything in the embedded SPA (`external/ui/**`) or in agent behaviour the chat
  exposes (new tools, modes, settings sections, prompts). Both plugins host the same UI, so
  both sets of users see it.
- **`editors/intellij/CHANGELOG.md` only** — changes under `editors/intellij/**` (tool
  window, Kotlin actions, JCEF workarounds, Gradle packaging).
- **`editors/vscode/CHANGELOG.md` only** — changes under `editors/vscode/**` (commands, menus,
  `foxxycode.*` settings, webview relay, VSIX packaging).

A change that a user cannot observe — refactoring, tests, CI, internal docs, a fix on one
editor's private plumbing — still produces a version bump on merge, but does not need its own
section. Fold it into the next entry that does have something to say, or leave the changelog
alone. Do **not** invent a user-facing line for work that has none.

## 4. The English twin (`CHANGELOG.en.md`)

The project website shows the recent change notes in Russian and English, so every section you
write in a `CHANGELOG.md` is written a second time, in English, in the `CHANGELOG.en.md` next to
it, in the same pull request. The editors never read the twins: `build.gradle.kts` renders only
the Russian file, and `editors/vscode/.vscodeignore` keeps the twin out of the VSIX.

- **Same headings from the top down.** Section *i* of the twin has the same version (or
  `Unreleased`) and the same date as section *i* of the Russian file. Renaming `Unreleased` to its
  tag, or adding your own section on top, happens in both files at once. The twin may stop
  earlier than the Russian file (releases before 0.2.78 were never translated) but may not run
  ahead of it.
- **Same entries.** Each paired section has the same number of `**Title.**` entries. Translate the
  meaning for an English reader rather than word for word.
- **In English.** Russian may appear only inside code or quotes, for example when an entry is about
  a label of the Russian interface (`«…»`).
- **No stubs, no conflict markers**, same as the Russian file.

`editors/intellij/changelog_twin_test.go` and the `CHANGELOG.en.md` block of
`editors/vscode/test/changelog.test.ts` enforce all of this.

## References

@AGENTS.md
@editors/intellij/CHANGELOG.md
@editors/vscode/CHANGELOG.md
@editors/intellij/CHANGELOG.en.md
@editors/vscode/CHANGELOG.en.md
