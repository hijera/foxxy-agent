"""Normalized three-way trials over committed Git blobs, without applying results."""

from bisect import bisect_left
from collections import Counter
import json
from pathlib import Path
import re
import subprocess
import tempfile

from upstream_branding import BRAND_REPLACEMENTS, CONTENT_REPLACEMENTS, normalize


def raw_git(repo: Path, *args: str, data: bytes | None = None) -> bytes:
    result = subprocess.run(["git", "-C", str(repo), *args], input=data, capture_output=True)
    if result.returncode:
        raise ValueError(result.stderr.decode("utf-8", errors="replace").strip())
    return result.stdout


def tree(repo: Path, rev: str) -> dict:
    entries = {}
    for entry in raw_git(repo, "ls-tree", "-r", "--full-tree", "-z", rev).split(b"\0"):
        if entry:
            header, path = entry.split(b"\t", 1)
            mode, kind, oid = header.decode("ascii").split()
            entries[path.decode("utf-8")] = {"mode": mode, "kind": kind, "oid": oid}
    return entries


def check_mappings(entries: dict, fork_path) -> None:
    mapped = {}
    for path in entries:
        destination = fork_path(path)
        if destination in mapped:
            raise ValueError(f"Rebranding collision: {mapped[destination]} and {path} -> {destination}")
        mapped[destination] = path
    for destination in mapped:
        parts = destination.split("/")
        for end in range(1, len(parts)):
            parent = "/".join(parts[:end])
            if parent in mapped:
                raise ValueError(f"Rebranding collision: {mapped[parent]} and {mapped[destination]} "
                                 f"-> file/directory overlap at {parent}")


def path_blockers(path: str, entries: set[str], ordered: list[str]) -> list[str]:
    parts = path.split("/")
    blockers = [parent for end in range(1, len(parts))
                if (parent := "/".join(parts[:end])) in entries]
    prefix = path + "/"
    index = bisect_left(ordered, prefix)
    while index < len(ordered) and ordered[index].startswith(prefix):
        blockers.append(ordered[index])
        index += 1
    return sorted(blockers)


def classify_path_conflicts(rows: list[dict], ours: dict, artifacts: dict) -> None:
    # A clean deletion or rename can make room for a file/directory transition.
    # Reconsider removals when a rename is itself blocked: its source must remain.
    while True:
        removed = {row["ours_path"] for row in rows if row["status"] == "delete_clean" or
                   (row["status"] in {"clean", "add_add_equal"} and
                    row["ours_path"] != row["fork_path"])}
        remaining = set(ours) - removed
        ordered = sorted(remaining)
        blocked = False
        for row in rows:
            if not row["objects"]["theirs"] or row["status"] == "path_conflict":
                continue
            blockers = path_blockers(row["fork_path"], remaining, ordered)
            if not blockers:
                continue
            row["status"] = "path_conflict"
            row["blocking_paths"] = blockers
            row["conflicts"] = 0
            row.pop("merged_mode", None)
            candidate = row["artifacts"].pop("merged", None)
            if candidate:
                artifacts.pop(candidate)
            blocked = True
        if not blocked:
            return


def read_blobs(repo: Path, objects: list[dict]) -> dict[str, bytes]:
    # Object IDs avoid cat-file's path quoting rules, including tabs and newlines.
    ids = sorted({obj["oid"] for obj in objects if obj and obj["kind"] == "blob"})
    if not ids:
        return {}
    output = raw_git(repo, "cat-file", "--batch", data=("\n".join(ids) + "\n").encode("ascii"))
    blobs, pos = {}, 0
    for oid in ids:
        end = output.index(b"\n", pos)
        header = output[pos:end].split()
        if len(header) != 3 or header[0].decode("ascii") != oid or header[1] != b"blob":
            raise ValueError(f"Cannot read preview blob {oid}")
        size = int(header[2])
        pos = end + 1
        blobs[oid] = output[pos:pos + size]
        if len(blobs[oid]) != size or output[pos + size:pos + size + 1] != b"\n":
            raise ValueError(f"Truncated preview blob {oid}")
        pos += size + 1
    return blobs


def merge_text(repo: Path, base: bytes, ours: bytes, theirs: bytes) -> tuple[bytes, int]:
    # Do not mistake markers already present in a transcript/fixture for new conflicts.
    longest = max((len(m[0]) for content in (base, ours, theirs)
                   for m in re.finditer(br"(?m)^[<|=>]+", content)), default=0)
    size = max(7, longest + 1)
    with tempfile.TemporaryDirectory(prefix="upstream-preview-") as directory:
        paths = [Path(directory) / name for name in ("ours", "base", "theirs")]
        for path, content in zip(paths, (ours, base, theirs)):
            path.write_bytes(content)
        result = subprocess.run(["git", "-C", str(repo), "merge-file", "-p", "--diff3",
                                 f"--marker-size={size}", "-L", "fork", "-L", "upstream-base",
                                 "-L", "upstream-target", *map(str, paths)], capture_output=True)
    # Git caps a successful conflict count at 127; 128+ (or a signal) is an error.
    if not 0 <= result.returncode <= 127:
        raise ValueError("git merge-file failed: " + result.stderr.decode("utf-8", errors="replace").strip())
    marker = b"<" * size + b" fork"
    count = sum(line == marker for line in result.stdout.splitlines())
    if bool(result.returncode) != bool(count):
        raise ValueError("git merge-file returned an inconsistent conflict result")
    return result.stdout, count


def build_preview(repo: Path, base: str, target: str, fork: str, git, changes, fork_path) -> tuple[dict, dict]:
    bases, targets, ours = (tree(repo, rev) for rev in (base, target, fork))
    check_mappings(bases, fork_path)
    check_mappings(targets, fork_path)
    rows = []
    for delta in changes(repo, base, target):
        path = delta["path"]
        old = delta.get("old_path", path)
        old_fork, destination = fork_path(old), fork_path(path)
        ours_path = old_fork if old_fork in ours else destination
        objects = {"base": bases.get(old), "ours": ours.get(ours_path), "theirs": targets.get(path)}
        collision = old_fork != destination and old_fork in ours and destination in ours
        if collision:
            objects["destination"] = ours[destination]
        rows.append({**delta, "fork_path": destination, "ours_path": ours_path,
                     "objects": objects, "rename_collision": collision})
    blobs = read_blobs(repo, [obj for row in rows for obj in row["objects"].values()])
    artifacts = {}
    for index, row in enumerate(rows):
        objects = row["objects"]
        contents = {side: blobs[obj["oid"]] for side, obj in objects.items() if obj and obj["kind"] == "blob"}
        row["artifacts"] = {}
        row["conflicts"] = 0

        def save(side, content, suffix):
            name = f"files/{index:05d}/{side}.{suffix}"
            artifacts[name] = content
            row["artifacts"][side] = name

        special = any(obj and obj["mode"] not in {"100644", "100755"} for obj in objects.values())
        binary = any(b"\0" in content for content in contents.values())
        try:
            normalized = {side: normalize(content, upstream=side in {"base", "theirs"},
                                          paths=(row["path"], row.get("old_path", row["path"])))
                          for side, content in contents.items()} if not binary and not special else None
        except UnicodeDecodeError:
            normalized = None
        for side, content in (normalized if normalized is not None else contents).items():
            save(side, content, "txt" if normalized is not None else "raw")
        if row.pop("rename_collision"):
            row["status"] = "rename_collision"
        elif special:
            row["status"] = "special_entry"
        elif binary:
            row["status"] = "binary"
        elif normalized is None:
            row["status"] = "non_utf8"
        else:
            b, o, t = (normalized.get(side) for side in ("base", "ours", "theirs"))
            modes = {side: obj["mode"] if obj else None for side, obj in objects.items()}
            if t is None:
                row["status"] = "already_absent" if o is None else (
                    "delete_clean" if o == b and modes["ours"] == modes["base"] else "delete_modify")
            elif o is None:
                row["status"] = "add_clean" if b is None else "modify_delete"
                if b is None:
                    save("merged", t, "txt")
                    row["merged_mode"] = modes["theirs"]
            elif modes["ours"] != modes["theirs"] and modes["ours"] != modes["base"] and modes["theirs"] != modes["base"]:
                row["status"] = "mode_conflict"
            else:
                merged, count = merge_text(repo, b or b"", o, t)
                row["conflicts"] = count
                row["status"] = ("add_add_conflict" if b is None else "conflict") if count else (
                    "add_add_equal" if b is None else "clean")
                row["merged_mode"] = modes["theirs"] if modes["ours"] == modes["base"] else modes["ours"]
                save("merged", merged, "txt")
    classify_path_conflicts(rows, ours, artifacts)
    report = {"schema_version": 1, "command": "preview", "base": base, "target": target, "fork_head": fork,
              "normalization": {"line_endings": "CRLF to LF on all text inputs",
                                "upstream_replacements": CONTENT_REPLACEMENTS,
                                "readme_content": "No branding replacements; review prose and examples manually",
                                "path_replacements": BRAND_REPLACEMENTS,
                                "fork_content": "No branding replacements; UTF-8 and LF normalization only"},
              "counts": dict(sorted(Counter(row["status"] for row in rows).items())),
              "conflicts": sum(row["conflicts"] for row in rows), "files": rows,
              "tests_executed": False,
              "warnings": ["Textual preview only: no patches applied, branches created, ledger updated or behavior verified.",
                           "Uses committed fork-base; uncommitted edits are excluded.",
                           "Covers the net range, including recorded ports; rename detection is heuristic.",
                           "Review semantic values, compatibility aliases and provenance after mechanical upstream rebranding.",
                           "Binary, non-UTF-8, special entries and structural conflicts require manual review."]}
    return report, artifacts


def preview_markdown(report: dict) -> str:
    lines = ["# Upstream three-way preview", "", f"Base: `{report['base']}`",
             f"Target: `{report['target']}`", f"Fork HEAD: `{report['fork_head']}`", ""]
    lines.extend("- " + warning for warning in report["warnings"])
    lines.extend(["", "## Dispositions", "", "| Status | Files |", "| --- | ---: |"])
    lines.extend(f"| {status} | {count} |" for status, count in report["counts"].items())
    lines.extend(["", f"Text conflict blocks: {report['conflicts']}", "", "## Paths", ""])
    for row in report["files"]:
        lines.append(f"- {json.dumps(row['path'], ensure_ascii=False)} -> "
                     f"{json.dumps(row['fork_path'], ensure_ascii=False)}: {row['status']} ({row['conflicts']} blocks)")
        if "old_path" in row:
            lines.append("  Renamed from: " + json.dumps(row["old_path"], ensure_ascii=False))
        if row.get("blocking_paths"):
            lines.append("  Blocked by: " + ", ".join(json.dumps(path, ensure_ascii=False)
                                                    for path in row["blocking_paths"]))
    return "\n".join(lines) + "\n"


def bundle_path(repo: Path, requested: Path, git, label: str = "Preview bundle") -> Path:
    path = requested if requested.is_absolute() else repo / requested
    if path.exists() or path.is_symlink():
        raise ValueError(f"{label} already exists: {path}")
    path = path.resolve()
    common = Path(git(repo, "rev-parse", "--path-format=absolute", "--git-common-dir").strip()).resolve()
    private = Path(git(repo, "rev-parse", "--absolute-git-dir").strip()).resolve()
    if any(path.is_relative_to(directory) for directory in (common, private)):
        raise ValueError(f"{label} cannot be inside Git metadata")
    return path


def write_bundle(path: Path, report: dict, artifacts: dict) -> None:
    path.mkdir(parents=True, exist_ok=False)
    report["bundle"] = str(path)
    try:
        for name, content in artifacts.items():
            destination = path / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            with destination.open("xb") as stream:
                stream.write(content)
        with (path / "preview.json").open("x", encoding="utf-8", newline="\n") as stream:
            stream.write(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        with (path / "preview.md").open("x", encoding="utf-8", newline="\n") as stream:
            stream.write(preview_markdown(report))
    except OSError as exc:
        raise ValueError(f"Incomplete preview bundle remains at {path}: {exc}") from exc
