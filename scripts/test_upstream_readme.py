"""README provenance survives every mechanical porting entry point."""

import json
from pathlib import Path

from test_upstream import GitRepoCase


README = ("# FoxxyCode\n\n"
          "> FoxxyCode is based on [coddy-agent](https://github.com/coddy-project/coddy-agent) by Coddy (MIT).\n"
          "> FoxxyCode основан на coddy-agent проекта Coddy.\n\n"
          "What FoxxyCode adds over coddy-agent / Что FoxxyCode добавляет к coddy-agent\n\n"
          "See https://github.com/EvilFreelancer/coddy-agent for upstream history.\n")
READMES = ("README", "README.md", "README.en.md", "editors/кириллица с пробелом/readme.ru.md",
           "cmd/coddy/README.md")


class ReadmeTests(GitRepoCase):
    def history(self):
        for path in READMES:
            self.write(path, README)
        self.write("cmd/coddy/tool.txt", "Coddy tool\n")
        self.commit("upstream documents and code")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", self.base)
        return target

    def assert_readmes(self, root):
        for path in READMES:
            destination = path.replace("cmd/coddy/", "cmd/foxxycode/")
            self.assertEqual((root / destination).read_text(encoding="utf-8"), README, path)
        self.assertEqual((root / "cmd/foxxycode/tool.txt").read_text(), "FoxxyCode tool\n")

    def test_prepared_patch_preserves_readme_provenance_and_still_rebrands_code(self):
        target = self.history()
        worktree = self.root / "prepared"
        report = json.loads(self.run_script("prepare", "--wave", "readme", "--from", self.base,
                            "--to", target, "--worktree", str(worktree), "--format", "json").stdout)
        self.git("-C", str(worktree), "apply", str(Path(report["bundle"]) / "adapted.patch"))
        self.assert_readmes(worktree)

    def test_preview_and_apply_preserve_readme_provenance(self):
        target = self.history()
        bundle = self.root / "preview"
        report = json.loads(self.run_script("preview", "--from", self.base, "--to", target,
                            "--bundle", str(bundle), "--format", "json").stdout)
        for row in report["files"]:
            if row["path"] in READMES:
                self.assertEqual((bundle / row["artifacts"]["theirs"]).read_text(encoding="utf-8"), README)
                self.assertEqual((bundle / row["artifacts"]["merged"]).read_text(encoding="utf-8"), README)
        self.run_script("apply", "--from", self.base, "--to", target)
        self.assert_readmes(self.root)

    def test_overlap_does_not_treat_self_attribution_as_an_already_ported_readme(self):
        target = self.history()
        broken = README.replace("https://github.com/coddy-project/coddy-agent", "https://github.com/hijera/foxxy-agent")
        broken = broken.replace("https://github.com/EvilFreelancer/coddy-agent", "https://github.com/hijera/foxxy-agent")
        self.write("README.md", broken.replace("coddy", "foxxycode").replace("Coddy", "FoxxyCode"))
        self.commit("accidental self attribution")
        report = json.loads(self.run_script("overlap", "--from", self.base, "--to", target,
                            "--format", "json").stdout)
        row = next(row for row in report["files"] if row["path"] == "README.md")
        self.assertEqual(row["status"], "differs")

    def test_existing_readme_still_receives_ordinary_upstream_changes(self):
        self.write("README.md", README + "\nFeature before\n")
        self.commit("upstream base")
        base = self.git("rev-parse", "HEAD").strip()
        self.write("README.md", README + "\nFeature after\n")
        self.commit("upstream update")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        self.run_script("apply", "--from", base, "--to", target)
        self.assertEqual((self.root / "README.md").read_text(encoding="utf-8"), README + "\nFeature after\n")

    def test_rebrand_check_accepts_nested_and_localized_readme_provenance(self):
        for path in READMES:
            self.write(path.replace("cmd/coddy/", "cmd/foxxycode/"), README)
        self.commit("README provenance")
        report = json.loads(self.run_script("rebrand-check", "--all", "--strict", "--format", "json").stdout)
        self.assertEqual(report["findings"], [])

    def test_readme_content_exception_does_not_hide_unported_code_paths(self):
        self.write("cmd/coddy/README.md", README)
        self.commit("upstream command path")
        report = json.loads(self.run_script("rebrand-check", "--all", "--format", "json", expected=1).stdout)
        self.assertEqual([(item["path"], item["kind"]) for item in report["findings"]],
                         [("cmd/coddy/README.md", "path")])
