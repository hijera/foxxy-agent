"""Ordered mechanical mappings, shared by patch and blob previews."""

import re

BRAND_REPLACEMENTS = {"coddy": "foxxycode", "Coddy": "FoxxyCode", "CODDY": "FOXXYCODE"}
MODULE_REPLACEMENTS = {
    "github.com/EvilFreelancer/coddy-agent": "github.com/hijera/foxxycode-agent",
    "github.com/coddy-project/coddy-agent": "github.com/hijera/foxxycode-agent",
}
# URLs name the repository, which differs from its historical Go module name.
REPOSITORY_REPLACEMENTS = {
    f"{scheme}://github.com/{owner}/coddy-agent": f"{scheme}://github.com/hijera/foxxy-agent"
    for scheme in ("https", "http") for owner in ("EvilFreelancer", "coddy-project")
}
CONTENT_REPLACEMENTS = {**REPOSITORY_REPLACEMENTS, **MODULE_REPLACEMENTS, **BRAND_REPLACEMENTS}
# The README files that carry attribution: readme, readme.md, readme.en.md and
# the like. Anything else that merely starts with "readme." (readme.go) is code.
README_NAME = re.compile(r"^readme(\.[a-z]{2})?\.(md|rst|txt)$|^readme$")


def content_replacements(*paths: str) -> dict:
    """README prose, provenance and examples need editorial review, not a rename."""
    for path in paths:
        name = path.replace("\\", "/").rsplit("/", 1)[-1].lower()
        if README_NAME.match(name):
            return {}
    return CONTENT_REPLACEMENTS


def fork_path(path: str) -> str:
    """Map only product tokens in paths; module owners are content mappings."""
    for source, replacement in BRAND_REPLACEMENTS.items():
        path = path.replace(source, replacement)
    return path


def normalize(data: bytes, upstream: bool, paths: tuple[str, ...] = ()) -> bytes:
    """Normalize UTF-8 Git blobs; never guess or replace undecodable bytes."""
    text = data.decode("utf-8").replace("\r\n", "\n")
    if upstream:
        for source, replacement in content_replacements(*paths).items():
            text = text.replace(source, replacement)
    return text.encode("utf-8")
