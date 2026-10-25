#!/usr/bin/env python3
"""Plan, preview, prepare and record upstream ports; Python 3.10+, Git and PyYAML."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import re
import subprocess
import sys

from upstream_ledger import OPEN_STATUSES, load_ledger
from upstream_ledger import STATUSES
from upstream_actions import prepare, record, record_batch
from upstream_apply import apply_markdown, apply_preview
from upstream_branding import BRAND_REPLACEMENTS, MODULE_REPLACEMENTS, REPOSITORY_REPLACEMENTS, fork_path
from upstream_overlap import build_overlap, overlap_markdown
from upstream_preview import build_preview, bundle_path, preview_markdown, write_bundle
from upstream_rebrand import build_report as build_rebrand, rebrand_markdown
from upstream_verify import build_plan, run_plan, verify_markdown
from upstream_wave import build_waves, wave_markdown


REGISTRY = ".cursor/rules/upstream-divergences.mdc"
MARKER = re.compile(r"//[^\n]*?fork\(([a-z0-9-]+)\)")


class PortError(Exception):
    """An actionable input or repository error."""


def git(repo: Path, *args: str) -> str:
    result = subprocess.run(["git", "-C", str(repo), *args], capture_output=True)
    if result.returncode:
        raise PortError(result.stderr.decode("utf-8", errors="replace").strip())
    return result.stdout.decode("utf-8", errors="replace")


def resolve(repo: Path, ref: str) -> str:
    try:
        return git(repo, "rev-parse", "--verify", "--end-of-options", ref + "^{commit}").strip()
    except PortError as exc:
        raise PortError(f"Cannot resolve {ref!r}. Check the ref and remote; fetch it explicitly first.") from exc


def blob(repo: Path, rev: str, path: str) -> str:
    # Paths come from Git or the fixed registry path, never from a shell expression.
    result = subprocess.run(["git", "-C", str(repo), "show", f"{rev}:{path}"], capture_output=True)
    if result.returncode:
        return ""
    return result.stdout.decode("utf-8", errors="replace")


def local_text(repo: Path, path: str) -> str:
    file = repo / path
    # Do not follow checkout symlinks out of the repository during inspection.
    if not file.resolve().is_relative_to(repo) or not file.is_file():
        return ""
    return file.read_text(encoding="utf-8", errors="replace")


def local_bytes(repo: Path, path: str) -> bytes | None:
    file = repo / path
    if not file.resolve().is_relative_to(repo) or not file.is_file():
        return None
    return file.read_bytes()


def changes(repo: Path, *args: str) -> list[dict]:
    return parse_changes(git(repo, "diff", "--no-ext-diff", "--no-textconv", "--name-status", "-z", "-M", *args, "--"))


def parse_changes(raw: str) -> list[dict]:
    fields = raw.split("\0")
    result = []
    cursor = 0
    while cursor < len(fields) and fields[cursor]:
        status, path = fields[cursor:cursor + 2]
        cursor += 2
        item = {"status": status, "path": path}
        if status.startswith(("R", "C")):
            item["old_path"] = path
            item["path"] = fields[cursor]
            cursor += 1
        result.append(item)
    return result


def paths_in(items: list[dict]) -> set[str]:
    return {item[key] for item in items for key in ("path", "old_path") if key in item}


def registry_entries(text: str) -> dict[str, dict]:
    entries = {}
    for line in text.splitlines():
        if not re.match(r"^\|\s*`[a-z0-9-]+`\s*\|", line):
            continue
        cells = [cell.strip() for cell in line.strip().strip("|").split("|")]
        if len(cells) != 7:
            raise PortError("Unexpected divergence registry row; expected seven columns: " + line)
        ident = cells[0].strip("`")
        entries[ident] = {
            "id": ident, "kind": cells[1],
            "sites": re.findall(r"`([^`]+)`", cells[4]),
            "tests": re.findall(r"`(Test\w+)`", cells[5]),
            "next_change": cells[6],
        }
    return entries


def risks(repo: Path, files: list[str], base: str, target: str | None,
          fork_ref: str | None = None) -> tuple[list[dict], list[str]]:
    old = blob(repo, base, REGISTRY)
    current = blob(repo, fork_ref, REGISTRY) if fork_ref else local_text(repo, REGISTRY)
    if not old and not current:
        raise PortError(f"Missing divergence registry: {REGISTRY}")
    # Retain old sites and tests if a port deleted or rewrote the registry itself.
    entries = registry_entries(old)
    for ident, entry in registry_entries(current).items():
        previous = entries.get(ident, {})
        entry["sites"] = sorted(set(entry["sites"] + previous.get("sites", [])))
        entry["tests"] = sorted(set(entry["tests"] + previous.get("tests", [])))
        entries[ident] = entry
    markers: dict[str, set[str]] = {}
    for path in files:
        if not path.endswith(".go") or path.endswith("_test.go"):
            continue
        texts = [blob(repo, base, path), blob(repo, fork_ref, path) if fork_ref else local_text(repo, path)]
        if target:
            texts.append(blob(repo, target, path))
        for ident in MARKER.findall("\n".join(texts)):
            markers.setdefault(ident, set()).add(path)
    touched = []
    for ident, entry in entries.items():
        hits = set(entry["sites"]).intersection(files) | markers.pop(ident, set())
        if hits:
            touched.append({**entry, "matched_files": sorted(hits)})
    warnings = [f"Unknown fork marker {ident}: {', '.join(sorted(paths))}" for ident, paths in sorted(markers.items())]
    if old and not current:
        warnings.append("Working-tree registry is missing; using the base revision for review.")
    deferred = [ident for ident, entry in entries.items() if entry["kind"] == "deferred"]
    if deferred:
        warnings.append("Review deferred entries manually (no file mapping): " + ", ".join(sorted(deferred)))
    return sorted(touched, key=lambda entry: entry["id"]), warnings


def obligations(files: list[str]) -> list[str]:
    result = [
        "Branding: rename coddy -> foxxycode, Coddy -> FoxxyCode and CODDY -> FOXXYCODE in code and paths, including identifiers, imports, strings, directories and filenames; reconcile references and tests.",
        "Run go test ./internal/forkguard -count=1 (registry integrity, not behavior).",
        "Run the listed guard tests in their owning packages with the required build tags; verify they actually execute.",
        "Finish with make test, make lint and make docs-check; report unavailable checks explicitly.",
    ]
    groups = [
        (("external/httpserver/",), "HTTP: align handlers, OpenAPI and docs/reference/http-api.md; add BDD coverage for changed behavior."),
        (("external/ui/",), 'UI: preserve i18n, follow DESIGN.md, rebuild with make build TAGS="http ui", and capture changed surfaces.'),
        (("internal/config/",), "Config: review schema, defaults, aliases, example, UI schema and bundled configuration skill; follow the schema publication rule."),
        (("cmd/foxxycode/", "internal/serve/"), "CLI: reconcile help, man page, completions and usage tests when commands or flags change."),
        (("internal/tools/fs/", "internal/textenc/"), "Encoding: preserve textenc.Decode/Encode and the original file encoding."),
        (("internal/mcp/", "internal/hooks/", "internal/subagents/"), "Trust: preserve project-scope gates and receipts; subagents cannot widen parent permissions."),
        (("editors/",), "Editors: run the affected plugin's own tests/build in addition to core checks."),
    ]
    for prefixes, instruction in groups:
        if any(path.startswith(prefixes) for path in files):
            result.append(instruction)
    if any("_windows" in path or path.startswith("internal/platform/") for path in files):
        result.append("Windows: run make check-windows and make lint-windows, plus applicable native tests.")
    result.append("Inspect build-tag and shared OS-signature changes manually; path hints do not cover every dependency.")
    return result


def build_report(repo: Path, args: argparse.Namespace) -> dict:
    ledger_path = args.ledger or Path("ports.yaml")
    if not ledger_path.is_absolute():
        ledger_path = repo / ledger_path
    ledger = load_ledger(ledger_path, required=args.ledger is not None or args.command == "ledger-check")
    if args.command == "ledger-check":
        return {"command": args.command, "ledger": ledger, "entries": len(ledger["ports"])}
    recorded = {entry["upstream_sha"]: entry for entry in ledger["ports"]}
    base = resolve(repo, args.from_ref if args.command == "scan" else args.base)
    commits = []
    if args.command == "scan":
        target = resolve(repo, args.to)
        try:
            git(repo, "merge-base", "--is-ancestor", base, target)
        except PortError as exc:
            raise PortError("--from must be an ancestor of --to; choose an explicit upstream range.") from exc
        files = set()
        for sha in git(repo, "rev-list", "--reverse", "--topo-order", f"{base}..{target}").splitlines():
            parents = git(repo, "rev-list", "--parents", "-n", "1", sha).split()[1:]
            if parents:
                delta = changes(repo, parents[0], sha)
            else:
                # A merged unrelated history can introduce another root commit.
                delta = parse_changes(git(repo, "diff-tree", "--root", "--no-commit-id",
                                          "--name-status", "-r", "-z", sha, "--"))
            files.update(paths_in(delta))
            commits.append({"sha": sha, "subject": git(repo, "show", "-s", "--format=%s", sha).strip(),
                            "merge": len(parents) > 1, "changes": delta})
        warning = "Ledger status is recorded evidence, not automatic verification. Unreviewed does not necessarily mean unported."
    else:
        target = None
        files = paths_in(changes(repo, base))
        files.update(filter(None, git(repo, "ls-files", "--others", "--exclude-standard", "-z").split("\0")))
        warning = "Review plan only: no tests executed. Includes committed, staged, unstaged and non-ignored untracked paths."
    ordered_files = sorted(files)
    for commit in commits:
        commit["port"] = recorded.get(commit["sha"], {"status": "unreviewed"})
    pending = [commit["sha"] for commit in commits if commit["port"]["status"] in OPEN_STATUSES | {"unreviewed"}]
    in_range = {commit["sha"] for commit in commits}
    backlog = [entry for entry in ledger["ports"] if entry["status"] in OPEN_STATUSES and entry["upstream_sha"] not in in_range]
    mappings = [{"upstream": path, "fork": fork_path(path)} for path in ordered_files
                if fork_path(path) != path]
    fork_files = sorted({fork_path(path) for path in ordered_files})
    review_files = sorted(files | set(fork_files))
    fork_ref = getattr(args, "fork_ref", None)
    divergences, warnings = risks(repo, review_files, base, target, fork_ref)
    warnings.insert(0, warning)
    if not ledger["present"]:
        warnings.append("No ports.yaml found; port status has not been recorded.")
    if ledger.get("notes"):
        warnings.append(ledger["notes"])
    if backlog:
        warnings.append("Backlog includes all open ledger entries outside this range; relevance to the target must be reviewed manually.")
    if args.command == "scan":
        warnings.append("Brand path mappings are review targets, not file edits. Inspect other renames, repository/module owners and mapping collisions manually.")
        warnings.append("Merge commits use their first-parent diff; related commits can overlap. Do not apply every listed patch blindly.")
    return {"schema_version": 1, "command": args.command, "base": base,
            "target": target, "fork_head": fork_ref or resolve(repo, "HEAD"), "commits": commits,
            "ledger": ledger, "pending_commits": pending, "backlog": backlog,
            "files": ordered_files, "fork_files": fork_files, "path_mappings": mappings,
            "branding": {"replacements": BRAND_REPLACEMENTS, "scope": "code and paths",
                         "module_replacements": MODULE_REPLACEMENTS, "repository_replacements": REPOSITORY_REPLACEMENTS},
            "divergences": divergences,
            "obligations": obligations(review_files), "warnings": warnings,
            "tests_executed": False}


def markdown(report: dict) -> str:
    if report["command"] == "preview":
        return preview_markdown(report)
    if report["command"] == "apply":
        return apply_markdown(report)
    if report["command"] == "overlap":
        return overlap_markdown(report)
    if report["command"] == "wave":
        return wave_markdown(report)
    if report["command"] == "rebrand-check":
        return rebrand_markdown(report)
    if report["command"] == "verify":
        return verify_markdown(report)
    if report["command"] in {"prepare", "record"}:
        return "```json\n" + json.dumps(report, ensure_ascii=False, indent=2) + "\n```\n"
    if report["command"] == "ledger-check":
        return f"Ledger valid: {report['entries']} entries. Commit existence and behavior were not verified.\n"
    lines = ["# Upstream " + report["command"], "", f"Base: `{report['base']}`",
             f"Target: `{report['target'] or 'working tree'}`", f"Fork HEAD: `{report['fork_head']}`", ""]
    for warning in report["warnings"]:
        lines.append("- " + warning)
    lines.extend(["", "## Candidate commits", ""])
    for item in report["commits"]:
        lines.append(f"- `{item['sha']}` [{item['port']['status']}] {'[merge] ' if item['merge'] else ''}{item['subject']}")
        if item["port"].get("reason"):
            lines.append("  Decision: " + item["port"]["reason"])
        for remaining in item["port"].get("remaining", []):
            lines.append("  Remaining: " + remaining)
    lines.extend(["", "## Open ledger entries outside this range", ""])
    for entry in report["backlog"]:
        lines.append(f"- `{entry['upstream_sha']}` [{entry['status']}]: {entry['reason']}")
        lines.extend("  Remaining: " + item for item in entry["remaining"])
    lines.extend(["", "## Changed paths", ""])
    lines.extend("- " + json.dumps(path, ensure_ascii=False) for path in report["files"])
    lines.extend(["", "## Branding in code and paths", "",
                  "Apply when porting: " + ", ".join(f"`{old}` -> `{new}`" for old, new in report["branding"]["replacements"].items()), ""])
    for item in report["path_mappings"]:
        lines.append(f"- {json.dumps(item['upstream'], ensure_ascii=False)} -> {json.dumps(item['fork'], ensure_ascii=False)}")
    lines.extend(["", "Content owner mappings (preview and prepared UTF-8 hunks):", ""])
    for mapping in (report["branding"]["module_replacements"], report["branding"]["repository_replacements"]):
        lines.extend(f"- `{source}` -> `{destination}`" for source, destination in mapping.items())
    lines.extend(["", "## Divergences to review", ""])
    if not report["divergences"]:
        lines.append("No direct match. This does not prove behavioral equivalence.")
    for entry in report["divergences"]:
        lines.extend([f"- **{entry['id']}**: {entry['next_change']}",
                      "  Files: " + ", ".join(entry["matched_files"]),
                      "  Guard tests: " + ", ".join(entry["tests"])])
    lines.extend(["", "## Required follow-up", ""])
    lines.extend("- [ ] " + item for item in report["obligations"])
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[1])
    commands = parser.add_subparsers(dest="command", required=True)
    scan = commands.add_parser("scan", help="Inventory an explicit upstream range without fetching or applying it")
    scan.add_argument("--from", dest="from_ref", required=True)
    scan.add_argument("--to", default="upstream/main")
    preview = commands.add_parser("preview", help="Preview normalized three-way merges of committed blobs; never applies results")
    preview.add_argument("--from", dest="from_ref", required=True)
    preview.add_argument("--to", default="upstream/main")
    preview.add_argument("--fork-base", default="HEAD")
    preview.add_argument("--bundle", type=Path, help="Save inputs, merged candidates and reports in a new directory")
    apply_command = commands.add_parser("apply", help="Write clean preview dispositions into the working tree; never stages or commits")
    apply_command.add_argument("--from", dest="from_ref", required=True)
    apply_command.add_argument("--to", default="upstream/main")
    apply_command.add_argument("--fork-base", default="HEAD")
    apply_command.add_argument("--dry-run", action="store_true")
    overlap = commands.add_parser("overlap", help="Report which net-range files already match the fork")
    overlap.add_argument("--from", dest="from_ref", required=True)
    overlap.add_argument("--to", default="upstream/main")
    overlap.add_argument("--fork-base", default="HEAD")
    wave = commands.add_parser("wave", help="Partition an upstream range into reviewable waves at release boundaries")
    wave.add_argument("--from", dest="from_ref", required=True)
    wave.add_argument("--to", default="upstream/main")
    wave.add_argument("--max-commits", type=int, default=60, help="Cut at a release tag once a wave reaches this many commits")
    wave.add_argument("--max-tags", type=int, default=5, help="Cut at a release tag once a wave covers this many tags")
    rebrand = commands.add_parser("rebrand-check", help="Flag leftover upstream branding; exit 1 on structural errors")
    rebrand.add_argument("--base", dest="from_ref", help="Scan the files changed since this fork revision")
    rebrand.add_argument("--all", action="store_true", help="Scan every tracked file")
    rebrand.add_argument("--strict", action="store_true", help="Also fail on notes, not only structural errors")
    rebrand.add_argument("paths", nargs="*", help="Files or directories to scan")
    verify = commands.add_parser("verify", help="Plan guard tests for a port review; --run executes them")
    verify.add_argument("--base", required=True)
    verify.add_argument("--run", action="store_true", help="Execute the planned go test commands")
    check = commands.add_parser("check", help="Plan review and checks for changes since a fork base; does not run tests")
    check.add_argument("--base", required=True)
    ledger_check = commands.add_parser("ledger-check", help="Validate ledger structure without fetching or verifying commits")
    for command in (scan, check, ledger_check, preview, apply_command, overlap, wave, rebrand, verify):
        command.add_argument("--format", choices=("markdown", "json"), default="markdown")
        command.add_argument("--output", type=Path, help="Create a UTF-8 report; refuse to overwrite an existing file")
    for command in (scan, check, ledger_check):
        command.add_argument("--ledger", type=Path, help="Ledger path relative to the inspected repository (default: ports.yaml)")
    prep = commands.add_parser("prepare", help="Create a new worktree and save original/adapted review patches; never applies them")
    prep.add_argument("--wave", required=True)
    prep.add_argument("--from", dest="from_ref", required=True)
    prep.add_argument("--to", default="upstream/main")
    prep.add_argument("--fork-base", default="HEAD")
    prep.add_argument("--worktree", type=Path, required=True)
    rec = commands.add_parser("record", help="Atomically record reviewed commits in an existing ledger")
    rec.add_argument("--batch", type=Path, help="JSON array of ledger entries to record in one atomic write")
    rec.add_argument("--upstream-commit")
    rec.add_argument("--status", choices=sorted(STATUSES))
    rec.add_argument("--fork-commit", action="append", default=[])
    rec.add_argument("--reason")
    rec.add_argument("--remaining", action="append", default=[])
    rec.add_argument("--update", action="store_true", help="Replace existing entries using all supplied fields")
    for command in (prep, rec):
        command.add_argument("--ledger", type=Path)
        command.add_argument("--dry-run", action="store_true")
        command.add_argument("--format", choices=("markdown", "json"), default="markdown")
    args = parser.parse_args()
    status = 0
    try:
        repo = Path(git(args.repo.resolve(), "rev-parse", "--show-toplevel").strip()).resolve()
        if args.command in {"preview", "apply", "overlap"}:
            base, target, fork = (resolve(repo, ref) for ref in (args.from_ref, args.to, args.fork_base))
            try:
                git(repo, "merge-base", "--is-ancestor", base, target)
            except PortError as exc:
                raise PortError("--from must be an ancestor of --to; choose an explicit upstream range.") from exc
            if args.command == "apply":
                report = apply_preview(repo, base, target, fork, git, changes, fork_path, dry_run=args.dry_run)
            elif args.command == "overlap":
                report = build_overlap(repo, base, target, fork, changes, fork_path)
            else:
                destination = bundle_path(repo, args.bundle, git) if args.bundle else None
                report, artifacts = build_preview(repo, base, target, fork, git, changes, fork_path)
                if destination:
                    write_bundle(destination, report, artifacts)
                else:
                    for row in report["files"]:
                        row.pop("artifacts")
        elif args.command == "wave":
            base, target = (resolve(repo, ref) for ref in (args.from_ref, args.to))
            try:
                git(repo, "merge-base", "--is-ancestor", base, target)
            except PortError as exc:
                raise PortError("--from must be an ancestor of --to; choose an explicit upstream range.") from exc
            ledger = load_ledger(repo / "ports.yaml")
            recorded = {entry["upstream_sha"]: entry for entry in ledger["ports"]}
            report = build_waves(repo, base, target, args.max_commits, args.max_tags, git, changes,
                                 paths_in, risks, recorded)
        elif args.command == "rebrand-check":
            report, status = rebrand_check(repo, args)
        elif args.command == "verify":
            report, status = verify_checks(repo, args)
        elif args.command == "prepare":
            report = prepare(repo, args, git, resolve, build_report, markdown, BRAND_REPLACEMENTS, fork_path, changes)
        elif args.command == "record":
            if args.batch:
                report = record_batch(repo, args, git, resolve)
            else:
                missing = [name for name, value in (("--upstream-commit", args.upstream_commit),
                                                    ("--status", args.status), ("--reason", args.reason))
                           if not value]
                if missing:
                    raise PortError("record needs --batch or " + ", ".join(missing))
                report = record(repo, args, git, resolve)
        else:
            report = build_report(repo, args)
        content = json.dumps(report, ensure_ascii=False, indent=2) + "\n" if args.format == "json" else markdown(report)
        if getattr(args, "output", None):
            with args.output.open("x", encoding="utf-8", newline="\n") as stream:
                stream.write(content)
        else:
            sys.stdout.write(content)
        return status
    except (PortError, OSError, ValueError) as exc:
        print(f"upstream: {exc}", file=sys.stderr)
        return 2


def rebrand_check(repo: Path, args) -> tuple[dict, int]:
    paths = []
    if args.paths:
        for name in args.paths:
            found = git(repo, "ls-files", "--", name).splitlines()
            if not found:
                raise PortError(f"No tracked files under {name!r}")
            paths.extend(found)
    elif args.from_ref:
        base = resolve(repo, args.from_ref)
        paths = sorted(paths_in(changes(repo, base)))
        paths.extend(filter(None, git(repo, "ls-files", "--others", "--exclude-standard", "-z").split("\0")))
    elif args.all:
        paths = [path for path in git(repo, "ls-files", "-z").split("\0") if path]
    else:
        raise PortError("rebrand-check needs explicit paths, --base <rev> or --all")
    warnings = [
        "Structural errors are paths, URLs and import/module references that keep upstream branding.",
        "Notes are bare mentions of the upstream name, including compatibility values such as"
        " `engine: coddy`, the compaction engine and provenance citations; review them by hand.",
        "Allowlisted paths keep naming the upstream project on purpose: the ledger, journal,"
        " registry rules, helper scripts, docs, README attribution and changelogs.",
        "Semantic values, aliases and repository owners outside the explicit mappings still need review.",
    ]
    report = build_rebrand(lambda path: local_bytes(repo, path), sorted(set(paths)),
                           scope=args.all and "all tracked files" or
                           (f"changes since {args.from_ref}" if args.from_ref else "explicit paths"),
                           warnings=warnings)
    failing = report["findings"] if args.strict else report["errors"]
    return report, 1 if failing else 0


def verify_checks(repo: Path, args) -> tuple[dict, int]:
    base = resolve(repo, args.base)
    files = paths_in(changes(repo, base))
    files.update(filter(None, git(repo, "ls-files", "--others", "--exclude-standard", "-z").split("\0")))
    matched, warnings = risks(repo, sorted(files | {fork_path(path) for path in files}), base, None, None)
    plan = build_plan(matched)
    report = {"schema_version": 1, "command": "verify", "base": base,
              "divergences": [entry["id"] for entry in matched],
              "commands": plan["commands"], "tests_executed": False,
              "warnings": ["Planning only unless --run is given; the helper does not run tests by default.",
                           "A successful go test that matched no tests is not validation; check no_tests results.",
                           "Registry integrity is not behavior: run the full regression before accepting a port."]
              + warnings}
    if not args.run:
        return report, 0
    report["commands"] = run_plan(plan["commands"], repo)
    report["tests_executed"] = True
    report["result"] = "passed" if all(item["status"] == "passed" for item in report["commands"]) else "failed"
    report["warnings"] = ["Executed commands report what actually ran; read the status of each command."]
    return report, 0 if report["result"] == "passed" else 1


if __name__ == "__main__":
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8")
    raise SystemExit(main())
