"""Rebrand-check is the gate that keeps upstream branding out of ported code."""

import json

from test_upstream import GitRepoCase


class RebrandTests(GitRepoCase):
    def report(self, *args, expected=0):
        result = self.run_script("rebrand-check", "--format", "json", *args, expected=expected)
        return json.loads(result.stdout)

    def test_flags_structural_leftovers_in_paths_imports_and_links(self):
        self.write("cmd/coddy/main.go", "package main\n")
        self.write("internal/x.go", 'import "github.com/coddy-project/coddy-agent/internal"\n')
        self.write("internal/link.go", "// see https://coddy.dev/docs for upstream docs\n")
        self.write("config.yaml", "engine: coddy\nmodel: keep\n")
        self.write("ports.yaml", "version: 1\nupstream: https://github.com/coddy-project/coddy-agent\nports: []\n")
        self.commit("leftovers")
        report = self.report("--all", expected=1)
        errors = {(item["path"], item["kind"]) for item in report["errors"]}
        self.assertEqual(errors, {("cmd/coddy/main.go", "path"),
                                  ("internal/x.go", "reference"),
                                  ("internal/link.go", "link")})
        notes = [item for item in report["findings"] if item["severity"] == "note"]
        self.assertEqual([item["path"] for item in notes], ["config.yaml"])
        self.assertNotIn("ports.yaml", report["scanned"])
        self.assertEqual(report["counts"]["errors"], 3)
        self.assertEqual(report["counts"]["notes"], 1)
        self.assertFalse(report["tests_executed"])

    def test_bare_mentions_are_notes_and_only_fail_under_strict(self):
        self.write("internal/config/schema.go", "package schema\n\n// engine: coddy stays canonical\n")
        self.write("internal/agent/prose.go", "package agent\n// The coddy engine replays the summary row.\n")
        self.write("internal/llm/auth.go", "package llm\nconst ClientID = \"coddy\"\n")
        self.commit("compatibility values")
        report = self.report("--all")
        self.assertEqual(report["errors"], [])
        self.assertEqual(report["counts"]["notes"], 3)
        self.report("--all", "--strict", expected=1)

    def test_compaction_engine_identifiers_and_issue_citations_are_notes(self):
        self.write("internal/agent/compact.go", "package agent\n"
                   "func use() bool { return a.cfg.Compaction.EngineIsCoddy() }\n")
        self.write("internal/config/engine.go", "package config\n"
                   "const CompactionEngineCoddy = \"coddy\"\n")
        self.write("external/httpserver/server_test.go", "package server\n"
                   "// covers coddy-project/coddy-agent#336:\n")
        self.write("internal/agent/helper.go", "package agent\nfunc coddyCompactedHistory() {}\n")
        self.write("external/ui/src/copy.ts", "export const help =\n"
                   "  'compaction \"coddy\"; skills from EvilFreelancer/rpa-skills'\n")
        self.commit("vocabulary")
        report = self.report("--all")
        self.assertEqual(report["errors"], [])
        self.assertEqual(report["counts"]["notes"], 5)

    def test_scopes_to_files_changed_since_the_given_base(self):
        self.write("internal/leftover.go", 'package leftover\nimport _ "github.com/coddy-project/coddy-agent"\n')
        self.commit("leftover")
        report = self.report("--base", self.base, expected=1)
        self.assertEqual([item["path"] for item in report["errors"]], ["internal/leftover.go"])
        self.assertEqual(report["scope"], "changes since " + self.base)
        self.git("rm", "-q", "internal/leftover.go")
        self.commit("remove leftover")
        self.assertEqual(self.report("--base", self.base)["findings"], [])

    def test_scans_explicit_paths_and_refuses_an_empty_scope(self):
        self.write("cmd/foxxycode/main.go", "package main\n")
        self.write("internal/other.go", 'package other\nimport _ "coddy-agent/pkg"\n')
        self.commit("two files")
        report = self.report("internal/other.go", expected=1)
        self.assertEqual([item["path"] for item in report["errors"]], ["internal/other.go"])
        self.run_script("rebrand-check", "--format", "json", expected=2)
        self.run_script("rebrand-check", "missing-dir", "--format", "json", expected=2)

    def test_allowlisted_paths_keep_naming_the_upstream(self):
        self.write(".claude/rules/upstream-divergences.md", "Keep the coddy spelling here only.\n")
        self.write("UPSTREAM_SYNC.md", "Sync through coddy 1.2.27.\n")
        self.write("README.md", "Based on [coddy-agent](https://github.com/coddy-project/coddy-agent).\n")
        self.write("editors/intellij/CHANGELOG.md", "Подготовка интерфейса к обновлениям Coddy.\n")
        self.write("docs/getting-started/foxxycode-and-coddy.md", "What FoxxyCode adds over coddy-agent.\n")
        self.commit("documents")
        report = self.report("--all")
        self.assertEqual(report["findings"], [])
        # The baseline registry plus the five documents written above.
        self.assertEqual(report["skipped_allowlisted"], 6)
