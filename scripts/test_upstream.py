"""Offline integration tests for the upstream port assistant."""

import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("upstream.py")


class GitRepoCase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Test")
        self.git("config", "user.email", "test@example.invalid")
        self.git("config", "core.autocrlf", "false")
        self.write("internal/agent/old name.go", "// fork(keep-budget): keep behavior\n")
        self.write(".cursor/rules/upstream-divergences.mdc", """# Registry
| ID | Kind | Upstream | FoxxyCode | Sites | Guard tests | Next upstream change |
|----|------|----------|-----------|-------|-------------|----------------------|
| `keep-budget` | code | old | keep | `internal/agent/old name.go` | `TestBudget` | Keep the budget. |
""")
        self.commit("baseline")
        self.base = self.git("rev-parse", "HEAD").strip()

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args], text=True, encoding="utf-8")

    def write(self, path, text):
        file = self.root / path
        file.parent.mkdir(parents=True, exist_ok=True)
        file.write_text(text, encoding="utf-8", newline="\n")

    def commit(self, message):
        self.git("add", ".")
        self.git("commit", "-qm", message)

    def run_script(self, *args, expected=0):
        result = subprocess.run([sys.executable, str(SCRIPT), "--repo", str(self.root), *args],
                                capture_output=True, text=True, encoding="utf-8")
        self.assertEqual(result.returncode, expected, result.stderr)
        return result


class UpstreamTests(GitRepoCase):
    def test_scan_tracks_renames_and_intermediate_reverted_changes(self):
        self.git("mv", "internal/agent/old name.go", "internal/agent/new name.go")
        self.commit("rename")
        self.write("external/ui/src/new.ts", "temporary\n")
        self.commit("temporary UI change")
        self.git("rm", "external/ui/src/new.ts")
        self.commit("revert UI change")
        before = self.git("status", "--porcelain")
        report = json.loads(self.run_script("scan", "--from", self.base, "--to", "HEAD", "--format", "json").stdout)
        self.assertEqual(len(report["commits"]), 3)
        self.assertIn("internal/agent/old name.go", report["files"])
        self.assertIn("internal/agent/new name.go", report["files"])
        self.assertIn("external/ui/src/new.ts", report["files"])
        self.assertEqual(report["divergences"][0]["id"], "keep-budget")
        self.assertEqual(report["divergences"][0]["tests"], ["TestBudget"])
        self.assertEqual(before, self.git("status", "--porcelain"))

    def test_check_recovers_deleted_marker_and_registry_from_base(self):
        self.write("internal/agent/old name.go", "// no marker\n")
        self.git("rm", ".cursor/rules/upstream-divergences.mdc")
        self.write("internal/config/new.go", "package config\n")
        report = json.loads(self.run_script("check", "--base", self.base, "--format", "json").stdout)
        self.assertEqual(report["divergences"][0]["id"], "keep-budget")
        self.assertIn("internal/config/new.go", report["files"])
        self.assertTrue(any("schema" in item for item in report["obligations"]))
        self.assertFalse(report["tests_executed"])

    def test_rejects_nonancestor_range(self):
        self.write("left.txt", "left")
        self.commit("left")
        left = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", self.base)
        self.write("right.txt", "right")
        self.commit("right")
        result = self.run_script("scan", "--from", left, "--to", "HEAD", expected=2)
        self.assertIn("ancestor", result.stderr)

    def test_merge_of_unrelated_history_includes_its_root_change(self):
        self.git("checkout", "-q", "--orphan", "imported-history")
        self.git("rm", "-rf", ".")
        self.write("imported.txt", "imported")
        self.commit("import root")
        imported = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", self.base)
        self.git("merge", "--allow-unrelated-histories", "--no-edit", imported)
        report = json.loads(self.run_script("scan", "--from", self.base, "--to", "HEAD", "--format", "json").stdout)
        root_commit = next(item for item in report["commits"] if item["sha"] == imported)
        self.assertEqual(root_commit["changes"], [{"status": "A", "path": "imported.txt"}])
        self.assertTrue(any(item["merge"] for item in report["commits"]))

    def test_invalid_ref_has_actionable_error(self):
        result = self.run_script("scan", "--from", self.base, "--to", "upstream/main", expected=2)
        self.assertIn("fetch", result.stderr)

    def test_scan_maps_brand_paths_to_fork_guards_and_cli_checks(self):
        self.write("cmd/coddy/main.go", "package main\n")
        self.write("internal/Coddy/CODDY.go", "package example\n")
        self.commit("upstream branded paths")
        self.write("cmd/foxxycode/main.go", "// fork(keep-budget): local implementation\n")
        report = json.loads(self.run_script("scan", "--from", self.base, "--to", "HEAD", "--format", "json").stdout)
        self.assertIn({"upstream": "cmd/coddy/main.go", "fork": "cmd/foxxycode/main.go"}, report["path_mappings"])
        self.assertIn({"upstream": "internal/Coddy/CODDY.go", "fork": "internal/FoxxyCode/FOXXYCODE.go"}, report["path_mappings"])
        self.assertIn("cmd/foxxycode/main.go", report["divergences"][0]["matched_files"])
        self.assertTrue(any(item.startswith("CLI:") for item in report["obligations"]))
        self.assertEqual(report["branding"]["replacements"]["coddy"], "foxxycode")
        self.assertEqual((self.root / "cmd/coddy/main.go").read_text(), "package main\n")

    def test_rebrand_maps_both_sides_of_a_rename(self):
        self.write("internal/coddy/old.go", "package example\n")
        self.commit("upstream before rename")
        start = self.git("rev-parse", "HEAD").strip()
        self.git("mv", "internal/coddy/old.go", "internal/coddy/new.go")
        self.commit("upstream rename")
        report = json.loads(self.run_script("scan", "--from", start, "--to", "HEAD", "--format", "json").stdout)
        self.assertIn("internal/foxxycode/old.go", report["fork_files"])
        self.assertIn("internal/foxxycode/new.go", report["fork_files"])

    def test_test_fixture_markers_are_not_reported_as_production_guards(self):
        self.write("scripts/fixture.py", "value = '// fork(fake): fixture'\n")
        self.write("internal/agent/fixture_test.go", "// fork(fake): fixture\n")
        report = json.loads(self.run_script("check", "--base", self.base, "--format", "json").stdout)
        self.assertFalse(any("fake" in warning for warning in report["warnings"]))

    def test_empty_range_and_exclusive_output(self):
        report = json.loads(self.run_script("scan", "--from", self.base, "--to", self.base, "--format", "json").stdout)
        self.assertEqual(report["commits"], [])
        output = self.root / "report.md"
        self.run_script("scan", "--from", self.base, "--to", self.base, "--output", str(output))
        original = output.read_bytes()
        self.run_script("scan", "--from", self.base, "--to", self.base, "--output", str(output), expected=2)
        self.assertEqual(output.read_bytes(), original)

    def test_ledger_marks_completed_commits_and_keeps_partial_backlog(self):
        self.write("new.txt", "new")
        self.commit("new change")
        target = self.git("rev-parse", "HEAD").strip()
        self.write("ports.yaml", f"""version: 1
upstream: https://github.com/coddy-project/coddy-agent
ports:
  - upstream_sha: '{target}'
    status: adapted
    fork_commits: ['{target}']
    reason: Ported with branding changes.
    remaining: []
  - upstream_sha: '{self.base}'
    status: partial
    fork_commits: ['{target}']
    reason: Only recovery was ported.
    remaining: ['Port provider status.']
""")
        report = json.loads(self.run_script("scan", "--from", self.base, "--to", target, "--format", "json").stdout)
        self.assertEqual(report["commits"][0]["port"]["status"], "adapted")
        self.assertEqual(report["pending_commits"], [])
        self.assertEqual(report["backlog"][0]["upstream_sha"], self.base)
        self.assertEqual(report["backlog"][0]["remaining"], ["Port provider status."])
        self.run_script("ledger-check")

    def test_ledger_rejects_invalid_records_and_duplicate_yaml_keys(self):
        cases = [
            "version: 1\nversion: 1\nupstream: x\nports: []\n",
            "version: 2\nupstream: x\nports: []\n",
            "version: 1\nupstream: x\nports: []\nunknown: true\n",
            f"version: 1\nupstream: x\nports:\n  - upstream_sha: '{self.base}'\n    status: partial\n    fork_commits: []\n    reason: Missing remaining work.\n    remaining: []\n",
            "version: 1\nupstream: x\nports: !!python/object:builtins.object {}\n",
        ]
        for content in cases:
            with self.subTest(content=content):
                self.write("ports.yaml", content)
                result = self.run_script("ledger-check", expected=2)
                self.assertIn("upstream:", result.stderr)

    def test_missing_explicit_ledger_fails_but_absent_default_is_allowed(self):
        self.run_script("scan", "--from", self.base, "--to", self.base)
        self.run_script("scan", "--from", self.base, "--to", self.base, "--ledger", "missing.yaml", expected=2)
        self.run_script("ledger-check", expected=2)

    def test_ledger_rejects_inconsistent_status_evidence_and_duplicate_commits(self):
        entry = {"upstream_sha": self.base, "status": "ported", "fork_commits": [self.base],
                 "reason": "Verified port.", "remaining": []}
        invalid = [
            [entry, entry],
            [{**entry, "fork_commits": []}],
            [{**entry, "upstream_sha": self.base[:8]}],
            [{**entry, "status": "done"}],
            [{**entry, "remaining": ["Unfinished work"]}],
            [{**entry, "reason": ""}],
        ]
        for ports in invalid:
            with self.subTest(ports=ports):
                self.write("ports.yaml", json.dumps({"version": 1, "upstream": "example", "ports": ports}))
                self.run_script("ledger-check", expected=2)


if __name__ == "__main__":
    unittest.main()
