from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
HOOK = REPO_ROOT / ".kimi-code" / "hooks" / "attach_rules.py"


class AttachRulesTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.rules_dir = self.root / "rules"
        self.rules_dir.mkdir()
        self.state_dir = self.root / "state"
        self.write_rule(
            "always.mdc",
            "Always follow the repository workflow.",
            always=True,
        )
        self.write_rule(
            "http.mdc",
            "Keep the OpenAPI document aligned with HTTP handlers.",
            globs="external/httpserver/**/*.go",
        )
        self.write_rule(
            "docs.mdc",
            "Update the documentation map.",
            globs="docs/**/*.md",
        )

    def write_rule(self, name: str, body: str, globs: str = "", always: bool = False) -> None:
        self.rules_dir.joinpath(name).write_text(
            "\n".join(
                [
                    "---",
                    f"description: {name}",
                    f"globs: {globs}",
                    f"alwaysApply: {str(always).lower()}",
                    "---",
                    body,
                ]
            ),
            encoding="utf-8",
        )

    def run_hook(self, payload: object, session_id: str = "probe") -> subprocess.CompletedProcess[str]:
        env = os.environ.copy()
        env["KIMI_CODE_RULES_DIR"] = str(self.rules_dir)
        env["KIMI_CODE_RULES_STATE_DIR"] = str(self.state_dir)
        if not isinstance(payload, str):
            payload = json.dumps(payload)
        return subprocess.run(
            [sys.executable, str(HOOK)],
            input=payload,
            text=True,
            capture_output=True,
            env=env,
            cwd=REPO_ROOT,
            check=False,
        )

    def output(self, result: subprocess.CompletedProcess[str]) -> dict:
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(result.stdout, result.stderr)
        return json.loads(result.stdout)

    def additional_context(self, result: subprocess.CompletedProcess[str]) -> str:
        return self.output(result)["hookSpecificOutput"]["additionalContext"]

    def test_session_start_injects_only_always_on_rules(self) -> None:
        result = self.run_hook(
            {"hook_event_name": "SessionStart", "session_id": "probe", "source": "startup"}
        )

        context = self.additional_context(result)
        self.assertIn("Always follow the repository workflow.", context)
        self.assertNotIn("Keep the OpenAPI document aligned", context)
        self.assertEqual(self.output(result)["hookSpecificOutput"]["hookEventName"], "SessionStart")

    def test_pre_tool_use_matches_recursive_glob_and_absolute_path(self) -> None:
        target = REPO_ROOT / "external" / "httpserver" / "server.go"
        result = self.run_hook(
            {
                "hook_event_name": "PreToolUse",
                "session_id": "probe",
                "tool_name": "Edit",
                "tool_input": {"path": str(target)},
            }
        )

        context = self.additional_context(result)
        self.assertIn("Keep the OpenAPI document aligned", context)
        self.assertNotIn("Always follow the repository workflow.", context)
        self.assertNotIn("Update the documentation map.", context)

    def test_write_uses_nested_path_fields(self) -> None:
        result = self.run_hook(
            {
                "hook_event_name": "PreToolUse",
                "session_id": "probe",
                "tool_name": "Write",
                "tool_input": {"nested": [{"file_path": "docs/contributing/example.md"}]},
            }
        )

        self.assertIn("Update the documentation map.", self.additional_context(result))

    def test_rule_is_sent_only_once_per_session(self) -> None:
        payload = {
            "hook_event_name": "PreToolUse",
            "session_id": "probe",
            "tool_name": "Edit",
            "tool_input": {"path": "external/httpserver/server.go"},
        }

        self.assertTrue(self.run_hook(payload).stdout)
        repeated = self.run_hook(payload)
        self.assertEqual(repeated.returncode, 0, repeated.stderr)
        self.assertEqual(repeated.stdout, "")

        payload["session_id"] = "other-session"
        self.assertTrue(self.run_hook(payload).stdout)

    def test_always_on_rules_are_not_repeated_before_edit(self) -> None:
        self.run_hook({"hook_event_name": "SessionStart", "session_id": "probe", "source": "startup"})
        result = self.run_hook(
            {
                "hook_event_name": "PreToolUse",
                "session_id": "probe",
                "tool_name": "Edit",
                "tool_input": {"path": "main.go"},
            }
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "")

    def test_paths_outside_repository_are_ignored(self) -> None:
        result = self.run_hook(
            {
                "hook_event_name": "PreToolUse",
                "session_id": "probe",
                "tool_name": "Edit",
                "tool_input": {"path": str(self.root / "external" / "httpserver" / "server.go")},
            }
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "")

    def test_malformed_input_and_unsupported_events_fail_open(self) -> None:
        for payload in ("not json", {"hook_event_name": "PostToolUse", "session_id": "probe"}):
            with self.subTest(payload=payload):
                result = self.run_hook(payload)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, "")


if __name__ == "__main__":
    unittest.main()
