"""Partition an upstream range into reviewable waves at release tag boundaries."""

import re

from upstream_ledger import OPEN_STATUSES

PULL_REQUEST = re.compile(r"Merge pull request #(\d+)")


def tags_in(repo, git, shas: set[str]) -> dict[str, list[str]]:
    found: dict[str, list[str]] = {}
    output = git(repo, "for-each-ref", "refs/tags",
                 "--format=%(refname:short)\t%(objecttype)\t%(objectname)\t%(*objectname)")
    for line in output.splitlines():
        name, kind, oid, peeled = line.split("\t")
        commit = peeled or oid
        if commit in shas:
            found.setdefault(commit, []).append(name)
    return found


def plan_cuts(ordered: list[str], boundaries: set[str], max_commits: int, max_tags: int,
              checkpoints: set[str]):
    """End indices of consecutive waves; a wave ends at a release tag once it is large enough."""
    cuts, start, boundary_count, orphan_cut = [], 0, 0, False
    for index, sha in enumerate(ordered):
        if sha in boundaries and sha in checkpoints:
            boundary_count += 1
        span = index - start + 1
        cut = sha in checkpoints and sha in boundaries and (span >= max_commits or boundary_count >= max_tags)
        if not cut and sha in checkpoints and span > max_commits * 2:
            cut, orphan_cut = True, True
        if cut or index == len(ordered) - 1:
            cuts.append(index)
            start, boundary_count = index + 1, 0
    return cuts, orphan_cut


def checkpoint_order(commits: list[dict], base: str, target: str) -> tuple[list[dict], set[str]]:
    """Keep merged branches with their first-parent checkpoint, so ranges tile."""
    by_sha = {item["sha"]: item for item in commits}
    descendants = set()
    for item in commits:
        if any(parent == base or parent in descendants for parent in item["parents"]):
            descendants.add(item["sha"])
    mainline, current = [], target
    while current in by_sha:
        mainline.append(current)
        parents = by_sha[current]["parents"]
        current = parents[0] if parents else ""
    ordered, seen = [], set()
    for checkpoint in reversed(mainline):
        stack = [(checkpoint, False)]
        while stack:
            sha, expanded = stack.pop()
            if sha not in by_sha or sha in seen:
                continue
            if expanded:
                seen.add(sha)
                ordered.append(by_sha[sha])
            else:
                stack.append((sha, True))
                stack.extend((parent, False) for parent in reversed(by_sha[sha]["parents"]))
    return ordered, set(mainline) & descendants


def build_waves(repo, base: str, target: str, max_commits: int, max_tags: int, git, changes,
                paths_in, risks, recorded: dict) -> dict:
    if max_commits < 1 or max_tags < 1:
        raise ValueError("--max-commits and --max-tags must be positive")
    # Git separates commits with a newline on top of the format's own NULs.
    packed = [field.lstrip("\n") for field in git(
        repo, "log", "--reverse", "--topo-order", "--format=%H%x00%s%x00%P%x00",
        f"{base}..{target}").split("\0")]
    ordered = [{"sha": packed[i], "subject": packed[i + 1], "parents": packed[i + 2].split()}
               for i in range(0, len(packed) - 2, 3) if packed[i]]
    ordered, checkpoints = checkpoint_order(ordered, base, target)
    shas = {item["sha"] for item in ordered}
    tags = tags_in(repo, git, shas)
    cuts, orphan_cut = plan_cuts([item["sha"] for item in ordered], set(tags), max_commits, max_tags, checkpoints)
    warnings = [
        "Waves are planning aids: they do not fetch, apply, record or verify anything.",
        "Cut points prefer release tag boundaries so a wave can be reported as one upstream checkpoint.",
        "A merge commit and its constituent commits can describe the same change; do not port both.",
        "Commit counts are not porting effort; read the divergence and ledger columns before scheduling.",
    ]
    if orphan_cut:
        warnings.append("Some wave was cut without a release tag boundary; name its checkpoint manually.")
    waves, start = [], 0
    for index in cuts:
        window = ordered[start:index + 1]
        start = index + 1
        wave_from = base if not waves else waves[-1]["to"]
        wave_to = window[-1]["sha"]
        files = set()
        for item in changes(repo, wave_from, wave_to):
            files.update(paths_in([item]))
        matched, _ = risks(repo, sorted(files), wave_from, wave_to, None)
        merges = [item for item in window if len(item["parents"]) > 1]
        pull_requests = []
        for merge in merges:
            number = PULL_REQUEST.search(merge["subject"])
            if not number:
                continue
            constituents = [line for line in git(repo, "rev-list", f"{merge['parents'][0]}..{merge['parents'][1]}"
                                                 ).splitlines() if line]
            pull_requests.append({"number": int(number.group(1)), "merge": merge["sha"],
                                  "commits": constituents})
        waves.append({
            "index": len(waves) + 1, "from": wave_from, "to": wave_to,
            "tags": sorted({name for item in window for name in tags.get(item["sha"], [])}),
            "commits": len(window), "merges": len(merges),
            "first_subject": window[0]["subject"], "last_subject": window[-1]["subject"],
            "files": len(files), "divergences": [entry["id"] for entry in matched],
            "unreviewed": sum(1 for item in window
                              if item["sha"] not in recorded
                              or recorded[item["sha"]].get("status") in OPEN_STATUSES),
            "pull_requests": pull_requests,
            "shas": [item["sha"] for item in window],
        })
    return {"schema_version": 1, "command": "wave", "base": base, "target": target,
            "max_commits": max_commits, "max_tags": max_tags,
            "waves": waves,
            "totals": {"waves": len(waves), "commits": len(ordered),
                       "files": len({path for item in changes(repo, base, target)
                                     for path in paths_in([item])})},
            "tests_executed": False, "warnings": warnings}


def wave_markdown(report: dict) -> str:
    lines = ["# Upstream wave plan", "", f"Base: `{report['base']}`", f"Target: `{report['target']}`",
             f"Max commits: `{report['max_commits']}`, max tags: `{report['max_tags']}`", ""]
    lines.extend("- " + warning for warning in report["warnings"])
    lines.extend(["", "| # | From | To | Tags | Commits | Files | Divergences | Unreviewed |",
                  "| ---: | --- | --- | --- | ---: | ---: | --- | ---: |"])
    for wave in report["waves"]:
        lines.append(f"| {wave['index']} | `{wave['from'][:12]}` | `{wave['to'][:12]}` | "
                     f"{', '.join(wave['tags']) or '-'} | {wave['commits']} | {wave['files']} | "
                     f"{', '.join(wave['divergences']) or '-'} | {wave['unreviewed']} |")
    lines.extend(["", "## Waves", ""])
    for wave in report["waves"]:
        lines.append(f"### Wave {wave['index']}: `{wave['to'][:12]}`")
        lines.append(f"- Range: `{wave['from']}` .. `{wave['to']}`")
        lines.append(f"- Tags: {', '.join(wave['tags']) or 'none'}")
        lines.append(f"- Commits: {wave['commits']} ({wave['merges']} merges);"
                     f" files: {wave['files']}; unreviewed: {wave['unreviewed']}")
        lines.append(f"- First: {wave['first_subject']}")
        lines.append(f"- Last: {wave['last_subject']}")
        if wave["pull_requests"]:
            lines.append("- Pull requests: " + ", ".join(
                f"#{item['number']} ({len(item['commits'])} commits)" for item in wave["pull_requests"]))
        lines.append("- Divergences: " + (", ".join(wave["divergences"]) or "none"))
        lines.append("- Command: `python scripts/upstream.py preview --from "
                     + wave["from"] + " --to " + wave["to"] + " --fork-base HEAD`")
    return "\n".join(lines) + "\n"
