"""Apply writes only clean dispositions, in disposable repositories."""

import json

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
