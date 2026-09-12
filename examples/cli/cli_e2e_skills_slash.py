#!/usr/bin/env python3
"""Skills: the fixture skill appears in the slash catalog and its token lands.

The console runs from a workdir that also carries ``.foxxycode/skills/foxxycode_project_demo``
(``examples/skills_fixture/foxxycode_project_demo``), so the ``${CWD}/.foxxycode/skills`` entry of
the demo config must list that project-local skill in the header ``[Skills]`` section
next to the home-installed ``foxxycode_slash_demo``. On the console the process cwd is the
session workspace, so this only covers the entry on that surface; the server-started-
elsewhere case of hijera/foxxy-agent#146 is exercised by the HTTP and ACP twins.
"""

from __future__ import annotations

import shutil
import sys
import tempfile
from pathlib import Path

from cli_tui_driver import CR, REPO_ROOT, FoxxyCodeTUI, install_skill_fixture, ok

DEMO_TOKEN = "DEMO_SKILL_TOKEN:z7k9-demo-slash"
PROJECT_SLASH_NAME = "foxxycode_project_demo"


def install_project_skill_fixture(workdir: Path) -> None:
    """Copy the project-local demo skill under <workdir>/.foxxycode/skills/."""
    src = REPO_ROOT / "examples" / "skills_fixture" / PROJECT_SLASH_NAME
    dst = workdir / ".foxxycode" / "skills" / PROJECT_SLASH_NAME
    if src.exists():
        shutil.copytree(src, dst, dirs_exist_ok=True)


def main() -> int:
    home = Path(tempfile.mkdtemp(prefix="foxxycode-cli-skills-home-"))
    work = Path(tempfile.mkdtemp(prefix="foxxycode-cli-skills-work-"))
    install_skill_fixture(home)
    install_project_skill_fixture(work)
    tui = FoxxyCodeTUI("skills", home=str(home), workdir=str(work))
    try:
        tui.wait_for("foxxycode v", timeout=30)
        tui.wait_for("foxxycode_slash_demo", timeout=20)  # header [Skills] section
        tui.wait_for(PROJECT_SLASH_NAME, timeout=20)  # project-local skill from <workdir>/.foxxycode/skills
        tui.type_text("/foxxycode_slash_demo run the demo")
        tui.send(CR)
        tui.wait_turn_started(timeout=30)
        tui.wait_idle(timeout=240)
        if DEMO_TOKEN not in tui.assistant_text():
            raise AssertionError("demo skill token missing from the assistant reply")
        return ok("cli_e2e_skills_slash")
    finally:
        tui.close()


if __name__ == "__main__":
    sys.exit(main())
