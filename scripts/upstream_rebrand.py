"""Flag leftover upstream branding in selected paths, as a porting gate."""

import re

from upstream_branding import BRAND_REPLACEMENTS

TOKENS = tuple(BRAND_REPLACEMENTS)
# Provenance, history and tooling that must keep naming the upstream project.
ALLOWED_PATHS = (
    "ports.yaml", "UPSTREAM_SYNC.md",
    ".cursor/", ".claude/", ".codex/",
    "scripts/upstream", "scripts/test_upstream",
    "docs/", "README.md", "README.en.md",
)
ALLOWED_NAMES = ("CHANGELOG",)
URL = re.compile(r"[a-z][a-z0-9+.-]*://\S*coddy\S*", re.IGNORECASE)
# An import path or module/repository name for the upstream project: never
# legitimate in fork code. Other projects of the same owners (rpa-skills) are not
# upstream branding.
REFERENCE = re.compile(r"(?i)github\.com/\S*coddy\S*|\bcoddy-agent\b|\bcoddy-project/"
                       r"|\bEvilFreelancer/coddy")
# A citation of an upstream issue or pull request is provenance, not a leftover.
PROVENANCE = re.compile(r"(?i)\b(coddy-project|EvilFreelancer)/coddy-agent#\d+")
# Structural leftovers break links, imports and paths; bare naming is review copy.
SEVERITY = {"path": "error", "link": "error", "reference": "error", "content": "note"}


def allowed(path: str) -> bool:
    normalized = path.replace("\\", "/")
    name = normalized.rsplit("/", 1)[-1]
    return (any(normalized == item or normalized.startswith(item) for item in ALLOWED_PATHS)
            or name.startswith(ALLOWED_NAMES))


def classify(line: str) -> str:
    if URL.search(line):
        return "link"
    if REFERENCE.search(PROVENANCE.sub("", line)):
        return "reference"
    return "content"


def scan_text(path: str, data: bytes) -> list[dict]:
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        return []
    findings = []
    for number, line in enumerate(text.splitlines(), 1):
        if not any(token.lower() in line.lower() for token in TOKENS):
            continue
        kind = classify(line)
        findings.append({"path": path, "kind": kind, "severity": SEVERITY[kind],
                         "line": number, "text": line.strip()[:200]})
    return findings


def build_report(read, paths: list[str], scope: str, warnings: list[str]) -> dict:
    """read(path) -> bytes or None when the path is not a regular file."""
    findings, skipped, scanned = [], 0, []
    for path in paths:
        if allowed(path):
            skipped += 1
            continue
        if any(token.lower() in path.lower() for token in TOKENS):
            findings.append({"path": path, "kind": "path", "severity": SEVERITY["path"],
                             "line": None, "text": "path keeps upstream branding"})
        data = read(path)
        if data is None:
            continue
        scanned.append(path)
        if b"\0" in data[:8192]:
            continue
        findings.extend(scan_text(path, data))
    errors = [item for item in findings if item["severity"] == "error"]
    return {"schema_version": 1, "command": "rebrand-check", "scope": scope,
            "findings": findings, "errors": errors, "scanned": scanned,
            "skipped_allowlisted": skipped,
            "counts": {"findings": len(findings), "errors": len(errors),
                       "notes": len(findings) - len(errors),
                       "paths": sum(1 for item in findings if item["kind"] == "path"),
                       "links": sum(1 for item in findings if item["kind"] == "link"),
                       "references": sum(1 for item in findings if item["kind"] == "reference"),
                       "content": sum(1 for item in findings if item["kind"] == "content")},
            "tests_executed": False, "warnings": warnings}


def rebrand_markdown(report: dict) -> str:
    lines = ["# Upstream branding check", "", f"Scope: {report['scope']}", ""]
    lines.extend("- " + warning for warning in report["warnings"])
    lines.extend(["", f"Scanned {len(report['scanned'])} files;"
                  f" {report['skipped_allowlisted']} allowlisted paths skipped.", ""])
    if not report["findings"]:
        lines.append("No leftover upstream branding found.")
        return "\n".join(lines) + "\n"
    for title, items in (("Errors", report["errors"]),
                         ("Notes", [item for item in report["findings"]
                                    if item["severity"] == "note"])):
        lines.extend(["", f"## {title} ({len(items)})", ""])
        if not items:
            lines.append("None.")
        for item in items:
            location = f"{item['path']}:{item['line']}" if item["line"] else item["path"]
            lines.append(f"- **{item['kind']}** `{location}`: {item['text']}")
    return "\n".join(lines) + "\n"
