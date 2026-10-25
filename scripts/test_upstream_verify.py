"""Verify plans guard tests and only executes them when asked."""

import json
import os
from pathlib import Path
import shutil

from test_upstream import GitRepoCase
from upstream_verify import build_plan, package_of, run_plan


class PlanTests(GitRepoCase):
    def test_plan_covers_registry_integrity_and_named_guard_tests(self):
        plan = build_plan([{"id": "keep-budget", "matched_files": ["internal/agent/old name.go"],
                            "tests": ["TestBudget", "TestKeep"]}])
        self.assertEqual(plan["commands"][0]["command"], ["go", "test", "./internal/forkguard", "-count=1"])
        self.assertEqual(plan["commands"][1]["command"],
                         ["go", "test", "./internal/agent", "-run", "^(TestBudget|TestKeep)$", "-count=1"])
        self.assertEqual(plan["commands"][1]["reason"], "Guard tests for divergence keep-budget.")

    def test_plan_runs_the_package_suite_when_no_guard_test_is_named(self):
        plan = build_plan([{"id": "unnamed", "matched_files": ["internal/session/turn.go"], "tests": []}])
        self.assertEqual(plan["commands"][1]["command"], ["go", "test", "./internal/session", "-count=1"])

    def test_package_of_maps_paths_to_go_packages(self):
        self.assertEqual(package_of("internal/agent/old name.go"), "./internal/agent")
        self.assertEqual(package_of("top.go"), ".")

    def test_run_plan_classifies_pass_failure_missing_and_no_tests(self):
        plan = build_plan([{"id": "keep-budget", "matched_files": ["internal/agent/x.go"], "tests": ["TestBudget"]}])
        outcomes = {
            "passed": (0, "ok\n", ""),
            "no_tests": (0, "testing\n\tno tests to run\n", ""),
            "failed": (1, "", "FAIL: TestBudget"),
        }
        for expected, outcome in outcomes.items():
            with self.subTest(status=expected):
                results = run_plan(plan["commands"], self.root, runner=lambda cmd, o=outcome: o)
                self.assertTrue(all(item["status"] == expected for item in results))

    def test_run_plan_reports_an_unavailable_tool(self):
        results = run_plan([{"command": ["definitely-not-a-tool", "test"], "packages": [], "tests": [],
                             "reason": "Probe."}], self.root)
        self.assertEqual(results[0]["status"], "unavailable")

    def test_cli_plans_without_running_and_fails_when_run_cannot_execute(self):
        report = json.loads(self.run_script("verify", "--base", self.base, "--format", "json").stdout)
        self.assertFalse(report["tests_executed"])
        self.assertEqual(report["commands"][0]["packages"], ["./internal/forkguard"])
        self.assertIn("does not run tests by default", report["warnings"][0])
        without_go = self.path_without_go()
        result = self.run_script("verify", "--base", self.base, "--run", "--format", "json",
                                 expected=1, env=without_go)
        executed = json.loads(result.stdout)
        self.assertTrue(executed["tests_executed"])
        self.assertEqual(executed["result"], "failed")
        self.assertTrue(all(item["status"] == "unavailable" for item in executed["commands"]))

    def path_without_go(self):
        empty = self.root / "empty-bin"
        empty.mkdir(exist_ok=True)
        git = shutil.which("git")
        self.assertIsNotNone(git, "git is required for the helper suite")
        path = str(empty) + os.pathsep + str(Path(git).parent)
        return {**os.environ, "PATH": path}
