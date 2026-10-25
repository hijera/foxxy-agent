"""Overlap compares normalized content without merging anything."""

import json

from test_upstream import GitRepoCase


class OverlapTests(GitRepoCase):
    def overlap(self, base, target, *args, expected=0):
        return self.run_script("overlap", "--from", base, "--to", target,
                               "--format", "json", *args, expected=expected)

    def test_overlap_reports_identical_absent_differs_and_deletes(self):
        self.write("cmd/coddy/same.txt", "coddy same\n")
        self.write("cmd/coddy/changed.txt", "coddy before\n")
        self.write("cmd/coddy/kept.txt", "coddy kept\n")
        self.commit("base files")
        base = self.git("rev-parse", "HEAD").strip()
        self.write("cmd/coddy/same.txt", "coddy same v2\n")
        self.write("cmd/coddy/changed.txt", "coddy after\n")
        self.git("rm", "-q", "cmd/coddy/kept.txt")
        self.write("added.txt", "coddy new\n")
        self.commit("upstream changes")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        (self.root / "cmd/foxxycode").mkdir(parents=True, exist_ok=True)
        for name in ("same", "changed", "kept"):
            self.git("mv", f"cmd/coddy/{name}.txt", f"cmd/foxxycode/{name}.txt")
        self.write("cmd/foxxycode/same.txt", "foxxycode same v2\n")
        self.write("cmd/foxxycode/changed.txt", "foxxycode before\n")
        self.write("cmd/foxxycode/kept.txt", "foxxycode kept\n")
        self.commit("fork rebrand")
        report = json.loads(self.overlap(base, target).stdout)
        statuses = {row["fork_path"]: row["status"] for row in report["files"]}
        self.assertEqual(statuses, {"cmd/foxxycode/same.txt": "identical",
                                    "cmd/foxxycode/changed.txt": "differs",
                                    "cmd/foxxycode/kept.txt": "remains_in_fork",
                                    "added.txt": "absent"})
        self.assertEqual(report["already_ported"], ["cmd/foxxycode/same.txt"])
        self.assertFalse(report["tests_executed"])

    def test_overlap_recognizes_a_rename_the_fork_already_made(self):
        self.write("old.txt", "coddy content\n")
        self.commit("base")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("mv", "old.txt", "new.txt")
        self.commit("upstream rename")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        self.git("rm", "-q", "old.txt")
        self.write("new.txt", "foxxycode content\n")
        self.commit("fork already renamed and rebranded")
        report = json.loads(self.overlap(base, target).stdout)
        row = report["files"][0]
        self.assertEqual(row["ours_path"], "new.txt")
        self.assertEqual(row["fork_path"], "new.txt")
        self.assertEqual(row["status"], "identical")
        self.assertEqual(report["already_ported"], ["new.txt"])

    def test_overlap_never_decides_binary_and_refuses_brand_path_collisions(self):
        (self.root / "asset.bin").write_bytes(b"\0coddy")
        self.commit("binary base")
        base = self.git("rev-parse", "HEAD").strip()
        (self.root / "asset.bin").write_bytes(b"\0coddy changed")
        self.commit("binary target")
        target = self.git("rev-parse", "HEAD").strip()
        report = json.loads(self.overlap(base, target).stdout)
        self.assertEqual(report["files"][0]["status"], "binary")
        self.assertEqual(report["already_ported"], [])
        self.write("coddy.txt", "a")
        self.write("foxxycode.txt", "b")
        self.commit("collision")
        self.overlap(base, "HEAD", expected=2)

    def test_overlap_marks_mode_only_differences(self):
        self.write("script.sh", "coddy echo\n")
        self.commit("base script")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("update-index", "--chmod=+x", "script.sh")
        self.commit("upstream executable")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        self.write("script.sh", "foxxycode echo\n")
        self.commit("fork rebranded content")
        report = json.loads(self.overlap(base, target).stdout)
        self.assertEqual(report["files"][0]["status"], "mode_only")
        self.assertEqual(report["already_ported"], ["script.sh"])
