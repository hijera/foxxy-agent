"""Explicit worktree preparation and atomic recording for upstream ports."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

from upstream_ledger import load_ledger, validate_ledger


def raw_git(repo: Path, *args: str) -> bytes:
    result = subprocess.run(["git", "-C", str(repo), *args], capture_output=True)
    if result.returncode:
        raise ValueError(result.stderr.decode("utf-8", errors="replace"))
    return result.stdout


def adapt_patch(patch: bytes, replacements: dict) -> tuple[bytes, list[str]]:
    """Keep binary payloads intact and remove stale text blob IDs (no --3way)."""
    parts = re.split(br"(?m)(?=^diff --git )", patch)
    output, warnings = [], []
    for part in parts:
        if not part:
            continue
        header = part.split(b"\n", 1)[0].decode("utf-8", errors="replace")
        binary = b"\nGIT binary patch\n" in part
        try:
            part.decode("utf-8")
            utf8 = True
        except UnicodeDecodeError:
            utf8 = False
        if binary or not utf8:
            warnings.append(f"Content left unchanged (binary or non-UTF-8): {header}")
        payload = False
        for line in part.splitlines(keepends=True):
            if line.startswith((b"GIT binary patch", b"@@")):
                payload = True
            # Text IDs describe pre-rebranding blobs and cannot support a 3-way merge.
            if not binary and line.startswith(b"index "):
                continue
            if not payload or (utf8 and not binary):
                for source, replacement in replacements.items():
                    line = line.replace(source.encode("ascii"), replacement.encode("ascii"))
            output.append(line)
    return b"".join(output), warnings


def prepare(repo: Path, args, git, resolve, build_report, markdown, replacements, fork_path) -> dict:
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", args.wave):
        raise ValueError("--wave must be a lowercase alphanumeric/hyphen name, at most 64 characters")
    branch = "codex/upstream-" + args.wave
    git(repo, "check-ref-format", "refs/heads/" + branch)
    if git(repo, "branch", "--list", branch).strip():
        raise ValueError(f"Branch already exists: {branch}")
    worktree = args.worktree
    if not worktree.is_absolute():
        worktree = repo / worktree
    # Refuse even empty directories: preparation never takes over existing work.
    if worktree.exists() or worktree.is_symlink():
        raise ValueError(f"Worktree path already exists: {worktree}")
    worktree = worktree.resolve()
    common_dir = Path(git(repo, "rev-parse", "--path-format=absolute", "--git-common-dir").strip())
    if worktree.is_relative_to(common_dir.resolve()):
        raise ValueError("Worktree cannot be inside Git metadata")
    fork_base = resolve(repo, args.fork_base)
    report = build_report(repo, argparse.Namespace(command="scan", from_ref=args.from_ref,
                          to=args.to, ledger=args.ledger, fork_ref=fork_base))
    for rev in (report["base"], report["target"]):
        mapped = {}
        for path in filter(None, git(repo, "ls-tree", "-r", "--name-only", "-z", rev).split("\0")):
            destination = fork_path(path)
            if destination in mapped:
                raise ValueError(f"Rebranding collision: {mapped[destination]} and {path} -> {destination}")
            mapped[destination] = path
    original = raw_git(repo, "diff", "--binary", "--full-index", "--no-renames", "--no-ext-diff",
                       "--no-textconv", report["base"], report["target"], "--")
    adapted, warnings = adapt_patch(original, replacements)
    result = {"command": "prepare", "dry_run": args.dry_run, "branch": branch,
              "worktree": str(worktree), "fork_base": fork_base,
              "upstream_base": report["base"], "upstream_target": report["target"],
              "warnings": warnings + ["Patches cover the net range, including recorded ports; review partial/completed entries before applying.",
                                      "Adapted text is a mechanical candidate: review aliases, semantic values and repository owners. Do not use --3way.",
                                      "Only the committed fork base is checked out; source worktree edits are not copied."]}
    if args.dry_run:
        return result
    git(repo, "worktree", "add", "-b", branch, "--", str(worktree), fork_base)
    try:
        git_dir = Path(git(worktree, "rev-parse", "--absolute-git-dir").strip())
        bundle = git_dir / "upstream-port"
        bundle.mkdir()
        result["bundle"] = str(bundle)
        (bundle / "original.patch").write_bytes(original)
        (bundle / "adapted.patch").write_bytes(adapted)
        (bundle / "scan.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        (bundle / "scan.md").write_text(markdown(report), encoding="utf-8")
        (bundle / "manifest.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    except (OSError, ValueError) as exc:
        raise ValueError(f"Worktree remains at {worktree} on {branch}; bundle creation failed: {exc}") from exc
    return result


def record(repo: Path, args, git, resolve) -> dict:
    try:
        import yaml
    except ImportError as exc:
        raise ValueError("record requires: python -m pip install -r scripts/requirements-upstream.txt") from exc

    path = args.ledger or Path("ports.yaml")
    if not path.is_absolute():
        path = repo / path
    if path.is_symlink() or not path.resolve().is_relative_to(repo):
        raise ValueError("record requires a regular ledger inside the inspected repository")
    entry = {"upstream_sha": resolve(repo, args.upstream_commit), "status": args.status,
             "fork_commits": list(dict.fromkeys(resolve(repo, ref) for ref in args.fork_commit)),
             "reason": args.reason, "remaining": args.remaining}
    head = resolve(repo, "HEAD")
    for sha in entry["fork_commits"]:
        git(repo, "merge-base", "--is-ancestor", sha, head)
    lock = path.with_name(path.name + ".lock")
    lock_fd = None
    temporary = None
    try:
        if not args.dry_run:
            lock_fd = os.open(lock, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        before = path.read_bytes()
        ledger = load_ledger(path, required=True)
        document = {key: value for key, value in ledger.items() if key not in {"path", "present"}}
        existing = next((item for item in document["ports"] if item["upstream_sha"] == entry["upstream_sha"]), None)
        if existing is not None and existing != entry and not args.update:
            raise ValueError("Entry already exists; use --update to replace the complete decision")
        document["ports"] = [entry if item is existing else item for item in document["ports"]]
        if existing is None:
            document["ports"].append(entry)
        validate_ledger(document)
        result = {"command": "record", "dry_run": args.dry_run, "ledger": str(path),
                  "changed": existing != entry, "entry": entry}
        if args.dry_run or existing == entry:
            return result
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", newline="\n", dir=path.parent,
                                         prefix=path.name + ".", suffix=".tmp", delete=False) as stream:
            temporary = Path(stream.name)
            yaml.safe_dump(document, stream, sort_keys=False, allow_unicode=True)
            stream.flush()
            os.fsync(stream.fileno())
        if path.read_bytes() != before:
            raise ValueError("Ledger changed during recording; retry after reviewing the other edit")
        os.replace(temporary, path)
        temporary = None
        return result
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)
        if lock_fd is not None:
            os.close(lock_fd)
            lock.unlink()
