"""Compare net upstream content with the fork to spot already ported files."""

from collections import Counter
import json

from upstream_branding import normalize
from upstream_preview import check_mappings, read_blobs, tree


def build_overlap(repo, base: str, target: str, fork: str, changes, fork_path) -> dict:
    bases, targets, ours = (tree(repo, rev) for rev in (base, target, fork))
    check_mappings(bases, fork_path)
    check_mappings(targets, fork_path)
    rows = []
    for delta in changes(repo, base, target):
        path = delta["path"]
        old = delta.get("old_path", path)
        old_fork, destination = fork_path(old), fork_path(path)
        ours_path = old_fork if old_fork in ours else destination
        rows.append({**delta, "fork_path": destination, "ours_path": ours_path,
                     "objects": {"base": bases.get(old), "ours": ours.get(ours_path),
                                 "theirs": targets.get(path)}})
    blobs = read_blobs(repo, [obj for row in rows for obj in row["objects"].values()])
    for row in rows:
        objects = row["objects"]
        contents = {side: blobs[obj["oid"]] for side, obj in objects.items()
                    if obj and obj["kind"] == "blob"}
        modes = {side: obj["mode"] if obj else None for side, obj in objects.items()}
        special = any(obj and obj["mode"] not in {"100644", "100755"} for obj in objects.values())
        binary = any(b"\0" in content for content in contents.values())
        try:
            normalized = {side: normalize(content, upstream=side in {"base", "theirs"})
                          for side, content in contents.items()} if not binary and not special else None
        except UnicodeDecodeError:
            normalized = None
        row.pop("objects")
        row["modes"] = modes
        o, t = ((normalized or {}).get(side) for side in ("ours", "theirs"))
        if special:
            row["status"] = "special_entry"
        elif binary:
            row["status"] = "binary"
        elif normalized is None:
            row["status"] = "non_utf8"
        elif t is None:
            row["status"] = "already_absent" if o is None else "remains_in_fork"
        elif o is None:
            row["status"] = "absent"
        elif o == t:
            row["status"] = "identical" if modes["ours"] == modes["theirs"] else "mode_only"
        else:
            row["status"] = "differs"
    report = {"schema_version": 1, "command": "overlap", "base": base, "target": target,
              "fork_head": fork,
              "counts": dict(sorted(Counter(row["status"] for row in rows).items())),
              "already_ported": sorted(row["fork_path"] for row in rows
                                       if row["status"] in {"identical", "mode_only"}),
              "files": rows, "tests_executed": False,
              "warnings": ["Content comparison only: identical means the fork already matches the"
                           " normalized upstream target, not that the port was reviewed.",
                           "Binary, non-UTF-8 and special entries are undecided and need manual review.",
                           "Covers the net range, including recorded ports; rename detection is heuristic."]}
    return report


def overlap_markdown(report: dict) -> str:
    lines = ["# Upstream overlap report", "", f"Base: `{report['base']}`",
             f"Target: `{report['target']}`", f"Fork HEAD: `{report['fork_head']}`", ""]
    lines.extend("- " + warning for warning in report["warnings"])
    lines.extend(["", "## Dispositions", "", "| Status | Files |", "| --- | ---: |"])
    lines.extend(f"| {status} | {count} |" for status, count in report["counts"].items())
    lines.extend(["", "## Already ported", ""])
    lines.extend("- " + json.dumps(path, ensure_ascii=False) for path in report["already_ported"])
    if not report["already_ported"]:
        lines.append("None.")
    lines.extend(["", "## Paths", ""])
    for row in report["files"]:
        lines.append(f"- {json.dumps(row['path'], ensure_ascii=False)} -> "
                     f"{json.dumps(row['fork_path'], ensure_ascii=False)}: {row['status']}")
    return "\n".join(lines) + "\n"
