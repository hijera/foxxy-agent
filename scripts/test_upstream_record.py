"""Batch recording writes many reviewed decisions in one atomic step."""

import json

from test_upstream import GitRepoCase


class RecordBatchTests(GitRepoCase):
    def ledger(self):
        self.write("ports.yaml", "version: 1\nupstream: example\nnotes: Keep this note.\nports: []\n")

    def batch(self, entries, *args, expected=0):
        self.write("batch.json", json.dumps(entries))
        return self.run_script("record", "--batch", "batch.json", "--format", "json",
                               *args, expected=expected)

    def entry(self, sha, status="skipped", reason="Redundant.", fork=(), remaining=()):
        return {"upstream_sha": sha, "status": status, "fork_commits": list(fork),
                "reason": reason, "remaining": list(remaining)}

    def test_batch_records_every_entry_in_one_write(self):
        self.ledger()
        self.write("other.txt", "x\n")
        self.commit("second")
        second = self.git("rev-parse", "HEAD").strip()
        report = json.loads(self.batch([self.entry(self.base), self.entry(second, "adapted",
                                                                        "Adapted.", (second,))]).stdout)
        self.assertEqual(report["changed"], 2)
        self.assertEqual(len(report["entries"]), 2)
        ledger = json.loads(self.run_script("ledger-check", "--format", "json").stdout)["ledger"]
        self.assertEqual(ledger["notes"], "Keep this note.")
        self.assertEqual([item["status"] for item in ledger["ports"]], ["skipped", "adapted"])

    def test_batch_is_all_or_nothing_against_existing_decisions(self):
        self.ledger()
        self.write("other.txt", "x\n")
        self.commit("second")
        second = self.git("rev-parse", "HEAD").strip()
        self.run_script("record", "--upstream-commit", self.base, "--status", "skipped",
                        "--reason", "First decision.")
        recorded = (self.root / "ports.yaml").read_bytes()
        self.batch([self.entry(second, reason="New one."), self.entry(self.base, reason="Contradicts.")],
                   expected=2)
        self.assertEqual((self.root / "ports.yaml").read_bytes(), recorded)
        report = json.loads(self.batch([self.entry(second, reason="New one."),
                                        self.entry(self.base, reason="Contradicts.")],
                                       "--update").stdout)
        self.assertEqual(report["changed"], 2)
        ledger = json.loads(self.run_script("ledger-check", "--format", "json").stdout)["ledger"]
        self.assertEqual(len(ledger["ports"]), 2)
        self.assertIn("Contradicts.", [item["reason"] for item in ledger["ports"]])
        self.assertIn("New one.", [item["reason"] for item in ledger["ports"]])

    def test_batch_rejects_duplicates_malformed_entries_and_bad_json(self):
        self.ledger()
        before = (self.root / "ports.yaml").read_bytes()
        self.batch([self.entry(self.base), self.entry(self.base)], expected=2)
        self.write("batch.json", json.dumps({"not": "a list"}))
        self.run_script("record", "--batch", "batch.json", "--format", "json", expected=2)
        self.write("batch.json", "{broken")
        self.run_script("record", "--batch", "batch.json", "--format", "json", expected=2)
        self.write("batch.json", json.dumps([{"upstream_sha": self.base, "status": "skipped"}]))
        self.run_script("record", "--batch", "batch.json", "--format", "json", expected=2)
        self.assertEqual((self.root / "ports.yaml").read_bytes(), before)

    def test_batch_dry_run_reports_the_entries_without_writing(self):
        self.ledger()
        before = (self.root / "ports.yaml").read_bytes()
        report = json.loads(self.batch([self.entry(self.base)], "--dry-run").stdout)
        self.assertTrue(report["dry_run"])
        self.assertEqual(report["changed"], 1)
        self.assertEqual((self.root / "ports.yaml").read_bytes(), before)

    def test_batch_refuses_entries_that_are_not_an_evidence_pair(self):
        self.ledger()
        before = (self.root / "ports.yaml").read_bytes()
        self.batch([self.entry(self.base, "partial", "Incomplete.", remaining=("Finish UI",))], expected=2)
        self.assertEqual((self.root / "ports.yaml").read_bytes(), before)
        report = json.loads(self.batch([self.entry(self.base, "deferred", "Deferred.",
                                                   remaining=("Finish UI",))]).stdout)
        self.assertEqual(report["changed"], 1)
        self.assertEqual(report["entries"][0]["remaining"], ["Finish UI"])
