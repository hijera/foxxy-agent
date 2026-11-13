"""Write clean preview dispositions into the checkout, never staging or committing."""

import json
import os

from upstream_preview import build_preview, classify_path_conflicts, tree


# Textual merges with no conflict and no structural surprise; anything else stays manual.
APPLYABLE = {"clean", "add_clean", "delete_clean", "add_add_equal"}


def apply_preview(repo, base: str, target: str, fork: str, git, changes, fork_path,
                  dry_run: bool = False) -> dict:
    if fork != git(repo, "rev-parse", "HEAD").strip():
        raise ValueError("apply requires --fork-base to match the checkout's current HEAD")
    report, artifacts = build_preview(repo, base, target, fork, git, changes, fork_path)
    warnings = [
        "Writes the working tree only: nothing is staged, committed or pushed.",
        "Clean dispositions are mechanical candidates; review semantic values,"
        " compatibility aliases and provenance after applying.",
        "Conflicted and structural rows are skipped for manual review, never"
        " written with conflict markers.",
        "Skipped paths with uncommitted local edits are left untouched.",
    ]
    if dry_run:
        warnings.insert(0, "Dry run: the checkout was not modified.")
    applied, skipped, ready = [], [], []
    for row in report["files"]:
        destination = row["fork_path"]
        sources = [destination]
        if "old_path" in row:
            sources.append(fork_path(row["old_path"]))
        if row["status"] not in APPLYABLE:
            skipped.append({"path": destination, "reason": row["status"]})
            continue
        if any((repo / path).resolve() != repo / path or (repo / path).is_symlink()
               for path in sources):
            skipped.append({"path": destination, "reason": "unsafe_path"})
            continue
        dirty = [path for path in sources if git(repo, "status", "--porcelain", "--ignored",
                                                "--untracked-files=all", "--", ":(literal)" + path).strip()]
        if dirty:
            skipped.append({"path": destination, "reason": "local_edits", "paths": dirty})
            continue
        ready.append(row)
    # A skipped deletion/rename must not make its source look vacant to a write.
    classify_path_conflicts(ready, tree(repo, fork), artifacts)
    ready_rows = []
    for row in ready:
        if row["status"] not in APPLYABLE:
            skipped.append({"path": row["fork_path"], "reason": row["status"]})
        else:
            ready_rows.append(row)
    if not dry_run:
        # All inputs are in memory and local edits have been checked. Remove the
        # old shapes before creating destinations, including rename sources.
        removals = {row["ours_path"] for row in ready_rows if row["status"] == "delete_clean" or
                    (row["objects"]["ours"] and row["ours_path"] != row["fork_path"])}
        for path in sorted(removals, key=lambda name: (-name.count("/"), name)):
            remove(repo / path)
    for row in ready_rows:
        destination = row["fork_path"]
        if row["status"] == "delete_clean":
            action, mode = "delete", None
        else:
            action, mode = "write", row.get("merged_mode")
        if not dry_run and action == "write":
            target_file = repo / destination
            target_file.parent.mkdir(parents=True, exist_ok=True)
            target_file.write_bytes(artifacts[row["artifacts"]["merged"]])
            if mode:
                os.chmod(target_file, 0o755 if mode == "100755" else 0o644)
        applied.append({"path": destination, "action": action, "mode": mode,
                        "status": row["status"]})
    counts = {}
    for item in applied:
        counts[item["action"]] = counts.get(item["action"], 0) + 1
    for item in skipped:
        key = "skipped_" + item["reason"]
        counts[key] = counts.get(key, 0) + 1
    return {"schema_version": 1, "command": "apply", "dry_run": dry_run,
            "base": base, "target": target, "fork_head": fork,
            "applied": applied, "skipped": skipped,
            "counts": dict(sorted(counts.items())), "tests_executed": False,
            "warnings": warnings}


def remove(path) -> None:
    path.unlink()
    parent = path.parent
    while parent != parent.parent and not any(parent.iterdir()):
        parent.rmdir()
        parent = parent.parent


def apply_markdown(report: dict) -> str:
    lines = ["# Upstream apply report", "", f"Base: `{report['base']}`",
             f"Target: `{report['target']}`", f"Fork HEAD: `{report['fork_head']}`",
             f"Dry run: `{report['dry_run']}`", ""]
    lines.extend("- " + warning for warning in report["warnings"])
    lines.extend(["", "## Counts", "", "| Disposition | Files |", "| --- | ---: |"])
    lines.extend(f"| {name} | {count} |" for name, count in report["counts"].items())
    lines.extend(["", "## Applied", ""])
    for item in report["applied"]:
        lines.append(f"- {item['action']}: {json.dumps(item['path'], ensure_ascii=False)}"
                     + (f" (mode {item['mode']})" if item["mode"] else ""))
    if not report["applied"]:
        lines.append("None.")
    lines.extend(["", "## Skipped", ""])
    for item in report["skipped"]:
        lines.append(f"- {json.dumps(item['path'], ensure_ascii=False)}: {item['reason']}")
    if not report["skipped"]:
        lines.append("None.")
    return "\n".join(lines) + "\n"
