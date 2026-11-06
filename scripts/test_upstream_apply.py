"""Apply writes only clean dispositions, in disposable repositories."""

import json
import os
from pathlib import Path
import subprocess
import tempfile

from test_upstream import GitRepoCase


class ApplyTests(GitRepoCase):
    def history(self, before, upstream, fork, path="cmd/coddy/file.txt"):
        self.write(path, before)
        self.commit("upstream base")
        base = self.git("rev-parse", "HEAD").strip()
        self.write(path, upstream)
        self.commit("upstream target")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        mapped = path.replace("coddy", "foxxycode")
        if mapped != path:
            (self.root / mapped).parent.mkdir(parents=True, exist_ok=True)
            self.git("mv", path, mapped)
        self.write(mapped, fork)
        self.commit("fork changes")
        return base, target

    def apply(self, base, target, *args, expected=0):
        return self.run_script("apply", "--from", base, "--to", target,
                               "--format", "json", *args, expected=expected)

    def test_apply_writes_clean_merges_and_deletes_without_staging(self):
        self.write("cmd/coddy/file.txt", "Coddy\nold\nkeep\nlast\n")
        self.write("gone.txt", "base\n")
        self.commit("upstream base")
        base = self.git("rev-parse", "HEAD").strip()
        self.write("cmd/coddy/file.txt", "Coddy\nnew\nkeep\nlast\n")
        self.git("rm", "-q", "gone.txt")
        self.commit("upstream target")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        (self.root / "cmd/foxxycode").mkdir(parents=True, exist_ok=True)
        self.git("mv", "cmd/coddy/file.txt", "cmd/foxxycode/file.txt")
        self.write("cmd/foxxycode/file.txt", "FoxxyCode\nold\nkeep\nfork\n")
        self.commit("fork changes")
        report = json.loads(self.apply(base, target).stdout)
        self.assertEqual((self.root / "cmd/foxxycode/file.txt").read_text(),
                         "FoxxyCode\nnew\nkeep\nfork\n")
        self.assertFalse((self.root / "gone.txt").exists())
        self.assertEqual(report["counts"], {"delete": 1, "write": 1})
        self.assertEqual(self.git("diff", "--cached", "--name-only").strip(), "")
        status = self.git("status", "--porcelain")
        self.assertIn("cmd/foxxycode/file.txt", status)
        self.assertIn("gone.txt", status)

    def test_apply_refuses_conflicts_and_dry_run_changes_nothing(self):
        base, target = self.history("Coddy old\n", "Coddy upstream\n", "FoxxyCode local\n")
        before = self.git("status", "--porcelain")
        report = json.loads(self.apply(base, target, "--dry-run").stdout)
        self.assertTrue(report["dry_run"])
        self.assertEqual(report["applied"], [])
        self.assertEqual(report["skipped"], [{"path": "cmd/foxxycode/file.txt", "reason": "conflict"}])
        self.assertEqual((self.root / "cmd/foxxycode/file.txt").read_text(), "FoxxyCode local\n")
        self.assertEqual(before, self.git("status", "--porcelain"))
        self.assertIn("Dry run", report["warnings"][0])

    def test_apply_skips_paths_with_uncommitted_local_edits(self):
        base, target = self.history("Coddy\nold\n", "Coddy\nnew\n", "FoxxyCode\nold\n")
        self.write("cmd/foxxycode/file.txt", "uncommitted work\n")
        report = json.loads(self.apply(base, target).stdout)
        self.assertEqual(report["applied"], [])
        self.assertEqual(report["skipped"][0]["reason"], "local_edits")
        self.assertEqual((self.root / "cmd/foxxycode/file.txt").read_text(), "uncommitted work\n")

    def test_apply_moves_a_renamed_file_and_removes_the_old_path(self):
        self.write("cmd/coddy/old.txt", "coddy content\n")
        self.commit("base")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("mv", "cmd/coddy/old.txt", "cmd/coddy/new.txt")
        self.commit("upstream rename")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        (self.root / "cmd/foxxycode").mkdir(parents=True, exist_ok=True)
        self.git("mv", "cmd/coddy/old.txt", "cmd/foxxycode/old.txt")
        self.write("cmd/foxxycode/old.txt", "FoxxyCode fork edits\n")
        self.commit("fork edits")
        report = json.loads(self.apply(base, target).stdout)
        self.assertEqual(report["applied"][0]["path"], "cmd/foxxycode/new.txt")
        self.assertEqual((self.root / "cmd/foxxycode/new.txt").read_text(), "FoxxyCode fork edits\n")
        self.assertFalse((self.root / "cmd/foxxycode/old.txt").exists())
        self.assertEqual(self.git("diff", "--cached", "--name-only").strip(), "")

    def test_apply_orders_directory_to_file_transition_before_writing(self):
        self.write("entry/old.txt", "old child\n")
        self.commit("base directory")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("rm", "entry/old.txt")
        self.write("entry", "new parent\n")
        self.commit("upstream directory becomes file")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        report = json.loads(self.apply(base, target).stdout)
        self.assertEqual((self.root / "entry").read_text(), "new parent\n")
        self.assertEqual(report["counts"], {"delete": 1, "write": 1})
        self.assertEqual(self.git("diff", "--cached", "--name-only").strip(), "")

    def test_apply_renames_into_source_directory(self):
        self.write("entry", "original\n")
        self.commit("base file")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("mv", "entry", "temporary")
        (self.root / "entry").mkdir()
        self.git("mv", "temporary", "entry/child.txt")
        self.commit("upstream rename into directory")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        report = json.loads(self.apply(base, target).stdout)
        self.assertEqual((self.root / "entry/child.txt").read_text(), "original\n")
        self.assertEqual(report["counts"], {"write": 1})

    def test_apply_skips_ignored_local_files_including_dry_run(self):
        self.write("local.txt", "upstream addition\n")
        self.commit("upstream addition")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", self.base)
        self.write(".gitignore", "local.txt\n")
        self.commit("fork ignore rule")
        self.write("local.txt", "precious ignored content\n")
        for flags in (("--dry-run",), ()):
            with self.subTest(flags=flags):
                report = json.loads(self.apply(self.base, target, *flags).stdout)
                self.assertEqual(report["applied"], [])
                self.assertEqual(report["skipped"][0]["reason"], "local_edits")
                self.assertEqual((self.root / "local.txt").read_text(), "precious ignored content\n")

    def test_apply_uses_literal_pathspecs_for_dirty_filenames(self):
        base, target = self.history("old\ncontext\nlast\n", "upstream\ncontext\nlast\n",
                                    "old\ncontext\nfork\n", "[ab].txt")
        self.write("[ab].txt", "local edits\n")
        report = json.loads(self.apply(base, target).stdout)
        self.assertEqual(report["applied"], [])
        self.assertEqual(report["skipped"][0]["reason"], "local_edits")
        self.assertEqual((self.root / "[ab].txt").read_text(), "local edits\n")

    def test_apply_rejects_fork_base_that_differs_from_current_head(self):
        base, target = self.history("old\n", "upstream\n", "committed fork work\n", "file.txt")
        self.apply(base, target, "--fork-base", base, expected=2)
        self.assertEqual((self.root / "file.txt").read_text(), "committed fork work\n")

    def test_apply_never_follows_an_untracked_symlink_parent(self):
        self.write("directory/file.txt", "upstream addition\n")
        self.commit("upstream addition")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", self.base)
        with tempfile.TemporaryDirectory() as external:
            outside = Path(external) / "file.txt"
            outside.write_text("external content\n")
            try:
                os.symlink(external, self.root / "directory", target_is_directory=True)
            except OSError as exc:
                if os.name != "nt":
                    self.skipTest(f"Directory symlinks unavailable: {exc}")
                # Junctions exercise redirected directories without symlink privileges.
                subprocess.run(["cmd", "/c", "mklink", "/J", str(self.root / "directory"), external],
                               check=True, capture_output=True)
            report = json.loads(self.apply(self.base, target).stdout)
            self.assertEqual(report["applied"], [])
            self.assertEqual(report["skipped"][0]["reason"], "unsafe_path")
            self.assertEqual(outside.read_text(), "external content\n")

    def test_apply_keeps_dirty_rename_source_as_a_blocker_for_another_write(self):
        self.write("entry", "original\n")
        self.commit("base file")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("mv", "entry", "elsewhere")
        self.write("entry/child.txt", "new content\n")
        self.commit("upstream rename and reuse")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        self.write("entry", "local edits\n")
        for flags in (("--dry-run",), ()):
            with self.subTest(flags=flags):
                report = json.loads(self.apply(base, target, *flags).stdout)
                self.assertEqual(report["applied"], [])
                self.assertEqual({row["path"]: row["reason"] for row in report["skipped"]},
                                 {"elsewhere": "local_edits", "entry/child.txt": "path_conflict"})
                self.assertEqual((self.root / "entry").read_text(), "local edits\n")
                self.assertFalse((self.root / "elsewhere").exists())
