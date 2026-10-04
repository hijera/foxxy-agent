"""Mutation tests use disposable repositories only."""

import json
from pathlib import Path

from test_upstream import GitRepoCase


class ActionTests(GitRepoCase):
    def ledger(self):
        self.write("ports.yaml", "version: 1\nupstream: example\nnotes: Keep this note.\nports: []\n")

    def test_prepare_patches_apply_after_rebranding_without_touching_source(self):
        self.write("cmd/coddy/file.txt", "Coddy uses CODDY and coddy.\n")
        (self.root / "asset.bin").write_bytes(b"\0coddy\xff")
        self.commit("upstream addition")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", self.base)
        self.write("local.txt", "preserve my uncommitted work")
        tree = self.root / "worktrees" / "wave"
        report = json.loads(self.run_script("prepare", "--wave", "demo", "--from", self.base,
                            "--to", target, "--worktree", str(tree), "--format", "json").stdout)
        self.assertEqual(self.git("rev-parse", "HEAD").strip(), self.base)
        self.assertEqual((self.root / "local.txt").read_text(), "preserve my uncommitted work")
        self.assertFalse((tree / "cmd/foxxycode/file.txt").exists())
        bundle = report["bundle"]
        self.git("-C", str(tree), "apply", str(Path(bundle) / "adapted.patch"))
        self.assertEqual((tree / "cmd/foxxycode/file.txt").read_text(), "FoxxyCode uses FOXXYCODE and foxxycode.\n")
        self.assertEqual((tree / "asset.bin").read_bytes(), b"\0coddy\xff")
        self.run_script("prepare", "--wave", "demo", "--from", self.base, "--to", target,
                        "--worktree", str(tree), expected=2)

    def test_prepare_dry_run_and_collision_do_not_create_branch(self):
        tree = self.root / "worktrees" / "wave"
        self.run_script("prepare", "--wave", "demo", "--from", self.base, "--to", self.base,
                        "--worktree", str(tree), "--dry-run")
        self.assertFalse(tree.exists())
        self.assertNotIn("codex/upstream-demo", self.git("branch", "--list"))
        self.write("coddy.txt", "a")
        self.write("foxxycode.txt", "b")
        self.commit("collision")
        self.run_script("prepare", "--wave", "demo", "--from", self.base, "--to", "HEAD",
                        "--worktree", str(tree), expected=2)
        self.assertFalse(tree.exists())

    def test_prepare_updates_existing_rebranded_text_and_preserves_legacy_bytes(self):
        self.write("coddy name.txt", "Coddy before\n")
        self.write("legacy.txt", "placeholder")
        (self.root / "legacy.txt").write_bytes(b"coddy \xe9 before\n")
        self.commit("upstream base")
        start = self.git("rev-parse", "HEAD").strip()
        self.write("coddy name.txt", "Coddy after\n")
        (self.root / "legacy.txt").write_bytes(b"coddy \xe9 after\n")
        self.commit("upstream edit")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", start)
        self.git("mv", "coddy name.txt", "foxxycode name.txt")
        self.write("foxxycode name.txt", "FoxxyCode before\n")
        self.commit("fork branding")
        tree = self.root / "worktrees" / "edited"
        result = json.loads(self.run_script("prepare", "--wave", "edited", "--from", start,
                            "--to", target, "--worktree", str(tree), "--format", "json").stdout)
        self.git("-C", str(tree), "apply", str(Path(result["bundle"]) / "adapted.patch"))
        self.assertEqual((tree / "foxxycode name.txt").read_text(), "FoxxyCode after\n")
        self.assertEqual((tree / "legacy.txt").read_bytes(), b"coddy \xe9 after\n")
        self.assertTrue(any("non-UTF-8" in warning for warning in result["warnings"]))

    def test_record_dry_run_update_and_invalid_evidence_preserve_ledger(self):
        self.ledger()
        path = self.root / "ports.yaml"
        before = path.read_bytes()
        common = ("record", "--upstream-commit", self.base, "--reason", "Reviewed.")
        self.run_script(*common, "--status", "deferred", "--remaining", "Port UI", "--dry-run")
        self.assertEqual(path.read_bytes(), before)
        self.run_script(*common, "--status", "deferred", "--remaining", "Port UI")
        recorded = path.read_bytes()
        self.run_script(*common, "--status", "ported", "--fork-commit", self.base, expected=2)
        self.assertEqual(path.read_bytes(), recorded)
        self.run_script(*common, "--status", "ported", "--fork-commit", "missing", "--update", expected=2)
        self.assertEqual(path.read_bytes(), recorded)
        self.run_script(*common, "--status", "ported", "--fork-commit", self.base, "--update")
        ledger = json.loads(self.run_script("ledger-check", "--format", "json").stdout)["ledger"]
        self.assertEqual(ledger["notes"], "Keep this note.")
        self.assertEqual(len(ledger["ports"]), 1)
        self.assertEqual(ledger["ports"][0]["status"], "ported")

    def test_prepare_reads_guards_from_committed_fork_base_not_dirty_checkout(self):
        self.write(".cursor/rules/upstream-divergences.mdc", "| `invalid` | malformed |\n")
        tree = self.root / "worktrees" / "pinned"
        result = json.loads(self.run_script("prepare", "--wave", "pinned", "--from", self.base,
                            "--to", self.base, "--fork-base", self.base, "--worktree", str(tree),
                            "--format", "json").stdout)
        report = json.loads((Path(result["bundle"]) / "scan.json").read_text())
        self.assertEqual(report["fork_head"], self.base)
        self.assertIn("malformed", (self.root / ".cursor/rules/upstream-divergences.mdc").read_text())

    def test_record_refuses_lock_and_fork_commit_outside_head(self):
        self.ledger()
        before = (self.root / "ports.yaml").read_bytes()
        self.write("x", "x")
        self.commit("other branch")
        other = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", self.base)
        self.ledger()
        self.run_script("record", "--upstream-commit", other, "--status", "ported",
                        "--fork-commit", other, "--reason", "Wrong branch", expected=2)
        self.write("ports.yaml.lock", "other writer")
        self.run_script("record", "--upstream-commit", self.base, "--status", "skipped",
                        "--reason", "Not needed", expected=2)
        self.assertEqual((self.root / "ports.yaml").read_bytes(), before)
