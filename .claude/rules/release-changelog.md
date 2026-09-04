---
description: Predict the post-merge version and write the user-facing changelog before opening a PR
paths:
  - "editors/**"
  - "**/CHANGELOG.md"
---

# Release version and changelog (before every PR)

Merging a PR into `main` releases immediately: `.github/workflows/tag-on-merge.yaml` tags the
merge commit and the plugin/binary workflows publish from that tag. There is no separate
release step where someone writes release notes afterwards — so the notes are part of the PR.

## 1. Do not guess the version — write `Unreleased`

CI takes the **highest SemVer tag on `origin`** and bumps the **patch**, so the version a PR
releases under depends on **what merges before it**. While your PR is open that number is not
knowable: if another PR merges first, yours moves up by one and any number you wrote down is
already wrong.

So do not write one. The newest section is headed:

```markdown
## Unreleased — 2026-09-04
```

`build.gradle.kts` replaces `Unreleased` with the version it is actually building — the tag
the release workflow just created — so the heading users see in **Change Notes** always
matches the plugin version they installed. Only the date is yours to write.

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
`## X.Y.Z — <its own date>`, then add your own `## Unreleased — <today>` above it. If nobody
does this the notes still ship correctly — the build stamps whatever is on top — the file
just keeps coarser history, so it is worth doing.

`editors/intellij/changelog_test.go` enforces the shape: at most one `## Unreleased`, and it
must be the newest section.

## 2. Write the entry

**`editors/intellij/CHANGELOG.md`** is the source of the **Change Notes** tab the IDE shows
in `Settings | Plugins`. `build.gradle.kts` parses it and renders the newest sections into
`plugin.xml`; the version being built is put first.

Section format, newest first — the top one unnumbered, the released ones numbered:

```markdown
## Unreleased — 2026-09-04

**Короткий заголовок изменения.**
Что именно изменилось для того, кто пользуется плагином, и почему это заметно.

## 0.2.23 — 2026-08-03
...
```

Rules for the text, enforced by `editors/intellij/changelog_test.go` (runs in `make test`):

- **In Russian.** This file is read by users, not by CI. An entry with no Cyrillic fails.
- **In human terms, not commits.** Describe the behaviour that changed and what the user
  will notice. Not "port upstream d00bcd7", not a bullet list of commit subjects.
- **Not a stub.** `TBD` or an empty section fails.
- **Versions strictly descending**, one section per released version.
- **No merge-conflict markers.** A conflicted changelog still parses and still reads as
  Russian, so nothing else catches it — and 0.2.46 shipped with a `=======` paragraph in the
  Change Notes tab. Resolving `editors/intellij/CHANGELOG.md` after a merge means picking
  both sides' sections, in descending order, not leaving the markers in.

Everything else in the repository — commit messages, `UPSTREAM_SYNC.md`, `docs/` — stays in
English per `AGENTS.md`. This file is the deliberate exception, because it is product copy
shown inside the IDE.

## 3. When a PR needs no entry

A change that a user cannot observe — refactoring, tests, CI, internal docs — still produces
a version bump on merge, but does not need its own section. Fold it into the next entry that
does have something to say, or leave the changelog alone. Do **not** invent a user-facing
line for work that has none.

## References

@AGENTS.md
@editors/intellij/CHANGELOG.md
