"""Strict, read-only loading of the upstream port ledger."""

from pathlib import Path
import re


STATUSES = {"ported", "adapted", "partial", "deferred", "skipped"}
OPEN_STATUSES = {"partial", "deferred"}
SHA = re.compile(r"(?:[0-9a-f]{40}|[0-9a-f]{64})\Z")


def load_ledger(path: Path, required: bool = False) -> dict:
    if not path.exists() and not required:
        return {"path": str(path), "present": False, "ports": []}
    try:
        import yaml
    except ImportError as exc:
        raise ValueError("Reading ports.yaml requires: python -m pip install -r scripts/requirements-upstream.txt") from exc

    class UniqueLoader(yaml.SafeLoader):
        pass

    def unique_mapping(loader, node):
        result = {}
        for key_node, value_node in node.value:
            key = loader.construct_object(key_node, deep=True)
            if not isinstance(key, str):
                raise ValueError("Ledger mapping keys must be strings")
            if key in result:
                raise ValueError(f"Duplicate YAML key: {key}")
            result[key] = loader.construct_object(value_node, deep=True)
        return result

    UniqueLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, unique_mapping)
    try:
        document = yaml.load(path.read_text(encoding="utf-8"), Loader=UniqueLoader)
    except yaml.YAMLError as exc:
        raise ValueError(f"Invalid YAML in {path}: {exc}") from exc
    validate_ledger(document)
    return {"path": str(path), "present": True, **document}


def validate_ledger(document: dict) -> None:
    """Validate the same schema for loaded and newly recorded decisions."""
    if not isinstance(document, dict):
        raise ValueError("Ledger must be a mapping")
    allowed = {"version", "upstream", "notes", "ports"}
    if set(document) - allowed:
        raise ValueError("Unknown ledger fields: " + ", ".join(sorted(set(document) - allowed)))
    if type(document.get("version")) is not int or document["version"] != 1:
        raise ValueError("Ledger version must be 1")
    if not isinstance(document.get("upstream"), str) or not document["upstream"].strip():
        raise ValueError("Ledger upstream must be a nonempty repository URL")
    if "notes" in document and not isinstance(document["notes"], str):
        raise ValueError("Ledger notes must be text")
    ports = document.get("ports")
    if not isinstance(ports, list):
        raise ValueError("Ledger ports must be a list")
    seen = set()
    fields = {"upstream_sha", "status", "fork_commits", "reason", "remaining"}
    for number, entry in enumerate(ports, 1):
        label = f"Ledger entry {number}"
        if not isinstance(entry, dict) or set(entry) != fields:
            raise ValueError(f"{label} requires exactly: {', '.join(sorted(fields))}")
        sha = entry["upstream_sha"]
        if not isinstance(sha, str) or not SHA.fullmatch(sha):
            raise ValueError(f"{label}: upstream_sha must be a full lowercase Git SHA")
        if sha in seen:
            raise ValueError(f"Duplicate upstream_sha: {sha}")
        seen.add(sha)
        status = entry["status"]
        if not isinstance(status, str) or status not in STATUSES:
            raise ValueError(f"{label}: invalid status {status!r}")
        for field in ("fork_commits", "remaining"):
            value = entry[field]
            if not isinstance(value, list) or any(not isinstance(item, str) or not item.strip() for item in value):
                raise ValueError(f"{label}: {field} must be a list of nonempty strings")
        if any(not SHA.fullmatch(commit) for commit in entry["fork_commits"]):
            raise ValueError(f"{label}: fork_commits must contain full lowercase Git SHAs")
        if status in {"ported", "adapted", "partial"} and not entry["fork_commits"]:
            raise ValueError(f"{label}: {status} requires fork_commits as evidence")
        if not isinstance(entry["reason"], str) or not entry["reason"].strip():
            raise ValueError(f"{label}: reason is required")
        if status in OPEN_STATUSES and not entry["remaining"]:
            raise ValueError(f"{label}: {status} requires remaining work")
        if status not in OPEN_STATUSES and entry["remaining"]:
            raise ValueError(f"{label}: completed/skipped entries cannot have remaining work")
