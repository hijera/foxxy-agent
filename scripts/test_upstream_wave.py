"""Waves cut an interleaved range into reviewable release-sized pieces."""

import json

from test_upstream import GitRepoCase


class WaveTests(GitRepoCase):
    def tagged(self, count, tag=True):
        shas = []
        for number in range(1, count + 1):
            self.write(f"file{number}.txt", f"content {number}\n")
            self.commit(f"change {number}")
            shas.append(self.git("rev-parse", "HEAD").strip())
            if tag:
                self.git("tag", f"v{number}")
        return shas

    def wave(self, *args, expected=0):
        return self.run_script("wave", "--from", self.base, "--to", "HEAD",
                               "--format", "json", *args, expected=expected)

    def test_wave_cuts_at_release_tag_boundaries(self):
        shas = self.tagged(3)
        report = json.loads(self.wave("--max-commits", "2", "--max-tags", "2").stdout)
        self.assertEqual([item["to"] for item in report["waves"]], [shas[1], shas[2]])
        self.assertEqual(report["waves"][0]["tags"], ["v1", "v2"])
        self.assertEqual(report["waves"][0]["commits"], 2)
        self.assertEqual(report["waves"][1]["tags"], ["v3"])
        self.assertEqual(report["waves"][0]["from"], self.base)
        self.assertEqual(report["waves"][1]["from"], shas[1])
        self.assertEqual(report["totals"], {"waves": 2, "commits": 3, "files": 3})
        self.assertFalse(report["tests_executed"])

    def test_wave_respects_a_small_commit_budget_per_tag(self):
        shas = self.tagged(3)
        report = json.loads(self.wave("--max-commits", "1", "--max-tags", "100").stdout)
        self.assertEqual([item["to"] for item in report["waves"]], shas)
        self.assertTrue(all(item["commits"] == 1 for item in report["waves"]))

    def test_wave_cuts_untagged_stretches_and_says_so(self):
        self.tagged(5, tag=False)
        report = json.loads(self.wave("--max-commits", "1", "--max-tags", "100").stdout)
        self.assertEqual([item["commits"] for item in report["waves"]], [3, 2])
        self.assertTrue(any("without a release tag" in item for item in report["warnings"]))

    def test_wave_groups_pull_request_merges_with_their_commits(self):
        self.write("base.txt", "base\n")
        self.commit("trunk")
        trunk = self.git("rev-parse", "--abbrev-ref", "HEAD").strip()
        self.git("checkout", "-qb", "feature")
        self.write("feature.txt", "feature\n")
        self.commit("feature work")
        feature = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", trunk)
        self.git("merge", "--no-ff", "-m", "Merge pull request #7 from test/feature", "feature")
        merge = self.git("rev-parse", "HEAD").strip()
        report = json.loads(self.wave("--max-commits", "50").stdout)
        pull_requests = report["waves"][0]["pull_requests"]
        self.assertEqual(pull_requests, [{"number": 7, "merge": merge, "commits": [feature]}])
        self.assertEqual(report["waves"][0]["merges"], 1)

    def test_wave_counts_divergence_contacts_and_unreviewed_commits(self):
        shas = self.tagged(1, tag=False)
        self.write("internal/agent/old name.go", "// fork(keep-budget): keep behavior\n// changed\n")
        self.commit("touches a divergence site")
        hot = self.git("rev-parse", "HEAD").strip()
        self.write("ports.yaml", "version: 1\nupstream: example\nports:\n"
                   f"- upstream_sha: {shas[0]}\n  status: skipped\n  fork_commits: []\n"
                   "  reason: Redundant.\n  remaining: []\n")
        report = json.loads(self.wave("--max-commits", "50").stdout)
        wave = report["waves"][0]
        self.assertEqual(wave["divergences"], ["keep-budget"])
        self.assertEqual(wave["commits"], 2)
        self.assertEqual(wave["unreviewed"], 1)
        self.assertEqual(wave["files"], 2)
        self.assertEqual(wave["last_subject"], "touches a divergence site")
        self.assertEqual(wave["shas"][-1], hot)

    def test_wave_rejects_a_nonancestor_range_and_a_small_budget(self):
        self.tagged(1)
        self.wave("--max-commits", "0", expected=2)
        self.run_script("wave", "--from", "HEAD", "--to", self.base, "--format", "json", expected=2)

    def test_wave_ranges_tile_merged_histories_at_usable_checkpoints(self):
        trunk = self.git("rev-parse", "--abbrev-ref", "HEAD").strip()
        self.write("trunk.txt", "trunk\n")
        self.commit("trunk change")
        self.git("tag", "v-trunk")
        self.git("checkout", "-qb", "feature", self.base)
        self.write("feature.txt", "feature\n")
        self.commit("feature change")
        self.git("tag", "v-feature")
        self.git("checkout", "-q", trunk)
        self.git("merge", "--no-ff", "-m", "Merge pull request #8 from test/feature", "feature")
        report = json.loads(self.wave("--max-commits", "1", "--max-tags", "1").stdout)
        covered = set()
        for wave in report["waves"]:
            self.git("merge-base", "--is-ancestor", wave["from"], wave["to"])
            actual = set(self.git("rev-list", wave["from"] + ".." + wave["to"]).splitlines())
            self.assertEqual(set(wave["shas"]), actual)
            self.assertFalse(covered & actual)
            covered.update(actual)
        self.assertEqual(covered, set(self.git("rev-list", self.base + "..HEAD").splitlines()))
