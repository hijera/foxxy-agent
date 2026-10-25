"""Three-way previews run against disposable committed histories only."""

import json
from pathlib import Path
from unittest.mock import patch

from test_upstream import GitRepoCase
from upstream_preview import merge_text


class PreviewTests(GitRepoCase):
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

    def preview(self, base, target, *args, expected=0):
        return self.run_script("preview", "--from", base, "--to", target,
                               "--format", "json", *args, expected=expected)

    def test_preview_merges_independent_changes_and_retains_inputs_without_touching_checkout(self):
        base, target = self.history("Coddy\nold\nkeep\nlast\n", "Coddy\nnew\nkeep\nlast\n",
                                    "FoxxyCode\nold\nkeep\nfork\n")
        self.write("cmd/foxxycode/file.txt", "uncommitted changes must survive\n")
        before = self.git("status", "--porcelain")
        bundle = self.root / "preview-output"
        report = json.loads(self.preview(base, target, "--bundle", str(bundle)).stdout)
        self.assertEqual(report["base"], base)
        self.assertEqual(report["target"], target)
        self.assertEqual(report["fork_head"], self.git("rev-parse", "HEAD").strip())
        row = report["files"][0]
        self.assertEqual(row["status"], "clean")
        self.assertEqual(row["fork_path"], "cmd/foxxycode/file.txt")
        self.assertEqual((bundle / row["artifacts"]["merged"]).read_text(), "FoxxyCode\nnew\nkeep\nfork\n")
        self.assertEqual((bundle / row["artifacts"]["ours"]).read_text(), "FoxxyCode\nold\nkeep\nfork\n")
        self.assertTrue((bundle / "preview.json").is_file())
        self.assertTrue((bundle / "preview.md").is_file())
        self.assertEqual((self.root / "cmd/foxxycode/file.txt").read_text(), "uncommitted changes must survive\n")
        self.assertEqual(before, self.git("status", "--porcelain", "--", "cmd"))
        self.assertNotIn("codex/upstream", self.git("branch", "--list"))

    def test_preview_reports_conflicts_and_normalizes_crlf(self):
        base, target = self.history("Coddy old\n", "Coddy upstream\r\n", "FoxxyCode local\r\n")
        bundle = self.root / "conflicts"
        report = json.loads(self.preview(base, target, "--bundle", str(bundle)).stdout)
        row = report["files"][0]
        self.assertEqual(row["status"], "conflict")
        self.assertEqual(row["conflicts"], 1)
        merged = (bundle / row["artifacts"]["merged"]).read_bytes()
        self.assertIn(b"<<<<<<< fork\n", merged)
        self.assertIn(b"||||||| upstream-base\nFoxxyCode old\n", merged)
        self.assertNotIn(b"\r", merged)

    def test_preview_classifies_additions_deletions_and_missing_fork_files(self):
        for name in ("delete.txt", "edited-delete.txt", "missing.txt"):
            self.write(name, "base\n")
        self.commit("base files")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("rm", "delete.txt", "edited-delete.txt")
        self.write("missing.txt", "upstream\n")
        self.write("added.txt", "new\n")
        self.write("collision.txt", "upstream addition\n")
        self.commit("target")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        self.git("rm", "missing.txt")
        self.write("edited-delete.txt", "fork\n")
        self.write("collision.txt", "fork addition\n")
        self.commit("fork")
        report = json.loads(self.preview(base, target).stdout)
        statuses = {row["path"]: row["status"] for row in report["files"]}
        self.assertEqual(statuses, {"added.txt": "add_clean", "collision.txt": "add_add_conflict",
                                   "delete.txt": "delete_clean", "edited-delete.txt": "delete_modify",
                                   "missing.txt": "modify_delete"})

    def test_preview_records_rename_paths_and_refuses_destination_collision(self):
        base, target = self.history("base\n", "upstream\n", "fork\n", "old.txt")
        self.git("checkout", "-q", "--detach", target)
        self.git("mv", "old.txt", "new.txt")
        self.commit("rename")
        renamed = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        self.write("new.txt", "fork already owns destination\n")
        self.commit("destination collision")
        # Use a pure rename range so Git can detect it without a similarity ambiguity.
        report = json.loads(self.preview(target, renamed).stdout)
        row = report["files"][0]
        self.assertEqual(row["old_path"], "old.txt")
        self.assertEqual(row["path"], "new.txt")
        self.assertEqual(row["status"], "rename_collision")

    def test_preview_preserves_fork_edits_across_upstream_and_already_ported_renames(self):
        self.write("old.txt", "base\n")
        self.commit("rename base")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("mv", "old.txt", "new.txt")
        self.commit("rename upstream")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        self.write("old.txt", "fork edits\n")
        self.commit("fork edits")
        for already_renamed in (False, True):
            with self.subTest(already_renamed=already_renamed):
                if already_renamed:
                    self.git("mv", "old.txt", "new.txt")
                    self.commit("fork already renamed")
                bundle = self.root / f"rename-{already_renamed}"
                report = json.loads(self.preview(base, target, "--bundle", str(bundle)).stdout)
                row = report["files"][0]
                self.assertEqual(row["status"], "clean")
                self.assertEqual(row["fork_path"], "new.txt")
                self.assertEqual((bundle / row["artifacts"]["merged"]).read_text(), "fork edits\n")

    def test_preview_never_decodes_binary_or_legacy_text_with_replacement(self):
        for name, data in [("image.bin", b"\0Coddy"), ("legacy.txt", b"coddy \xe9")]:
            (self.root / name).write_bytes(data)
        self.commit("base bytes")
        base = self.git("rev-parse", "HEAD").strip()
        (self.root / "image.bin").write_bytes(b"\0Coddy changed")
        (self.root / "legacy.txt").write_bytes(b"coddy \xe9 changed")
        self.commit("changed bytes")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        bundle = self.root / "bytes"
        report = json.loads(self.preview(base, target, "--bundle", str(bundle)).stdout)
        rows = {row["path"]: row for row in report["files"]}
        self.assertEqual(rows["image.bin"]["status"], "binary")
        self.assertEqual(rows["legacy.txt"]["status"], "non_utf8")
        self.assertEqual((bundle / rows["legacy.txt"]["artifacts"]["theirs"]).read_bytes(), b"coddy \xe9 changed")
        self.assertNotIn("merged", rows["legacy.txt"]["artifacts"])

    def test_preview_maps_module_owners_and_repository_urls_explicitly(self):
        base, target = self.history("old\n", 'import "github.com/coddy-project/coddy-agent/internal/config"\n'
                                    'https://github.com/EvilFreelancer/coddy-agent\n', "old\n")
        bundle = self.root / "owners"
        report = json.loads(self.preview(base, target, "--bundle", str(bundle)).stdout)
        row = report["files"][0]
        merged = (bundle / row["artifacts"]["merged"]).read_text()
        self.assertIn('"github.com/hijera/foxxycode-agent/internal/config"', merged)
        self.assertIn("https://github.com/hijera/foxxy-agent", merged)

    def test_preview_refuses_existing_bundle_and_brand_collisions_without_changes(self):
        existing = self.root / "existing"
        existing.mkdir()
        (existing / "keep").write_text("keep")
        self.preview(self.base, self.base, "--bundle", str(existing), expected=2)
        self.assertEqual((existing / "keep").read_text(), "keep")
        self.write("coddy.txt", "a")
        self.write("foxxycode.txt", "b")
        self.commit("brand collision")
        output = self.root / "not-created"
        result = self.preview(self.base, "HEAD", "--bundle", str(output), expected=2)
        self.assertIn("collision", result.stderr.lower())
        self.assertFalse(output.exists())

    def test_preview_pins_fork_base_and_rejects_nonancestor_range(self):
        base, target = self.history("base\n", "upstream\n", "fork\n", "file.txt")
        report = json.loads(self.preview(base, target, "--fork-base", base).stdout)
        self.assertEqual(report["fork_head"], base)
        self.assertEqual(report["files"][0]["status"], "clean")
        self.preview(target, base, expected=2)

    def test_preview_does_not_write_without_bundle_and_preserves_existing_report(self):
        before = self.git("status", "--porcelain")
        report = json.loads(self.preview(self.base, self.base).stdout)
        self.assertEqual(report["counts"], {})
        self.assertEqual(report["files"], [])
        self.assertEqual(before, self.git("status", "--porcelain"))
        output = self.root / "report.md"
        self.run_script("preview", "--from", self.base, "--to", self.base, "--output", str(output))
        original = output.read_bytes()
        self.run_script("preview", "--from", self.base, "--to", self.base, "--output", str(output), expected=2)
        self.assertEqual(output.read_bytes(), original)
        self.preview(self.base, self.base, "--bundle", str(self.root / ".git" / "preview"), expected=2)
        self.assertFalse((self.root / ".git" / "preview").exists())

    def test_preview_keeps_fork_compatibility_values_and_unicode_paths(self):
        base, target = self.history("old\nunchanged\nlast\n", "new\nunchanged\nlast\n",
                                    "old\nunchanged\nengine: coddy\n", "name with space \u044f.txt")
        bundle = self.root / "unicode"
        report = json.loads(self.preview(base, target, "--bundle", str(bundle)).stdout)
        row = report["files"][0]
        self.assertEqual(row["path"], "name with space \u044f.txt")
        merged = (bundle / row["artifacts"]["merged"]).read_text(encoding="utf-8")
        self.assertEqual(merged, "new\nunchanged\nengine: coddy\n")

    def test_preview_classifies_symlinks_without_materializing_them(self):
        self.write("file.txt", "before\n")
        self.commit("regular base")
        base = self.git("rev-parse", "HEAD").strip()
        oid = self.git("hash-object", "file.txt").strip()
        self.git("update-index", "--add", "--cacheinfo", f"120000,{oid},link")
        self.git("commit", "-qm", "symlink in Git only")
        target = self.git("rev-parse", "HEAD").strip()
        bundle = self.root / "special"
        report = json.loads(self.preview(base, target, "--bundle", str(bundle)).stdout)
        row = report["files"][0]
        self.assertEqual(row["status"], "special_entry")
        self.assertEqual(row["objects"]["theirs"]["mode"], "120000")
        self.assertFalse((bundle / "link").exists())
        self.assertNotIn("merged", row["artifacts"])

    def test_merge_errors_are_not_reported_as_conflicts(self):
        with patch("upstream_preview.subprocess.run") as run:
            run.return_value.returncode = 128
            run.return_value.stderr = b"merge failure"
            with self.assertRaisesRegex(ValueError, "merge failure"):
                merge_text(self.root, b"base", b"ours", b"theirs")

    def test_preview_counts_more_than_127_conflicts_without_counting_input_markers(self):
        common = lambda i: "".join(f"context {i} {j}\n" for j in range(8))
        before = "<<<<<<< fork\n" + "".join(f"base {i}\n" + common(i) for i in range(130))
        ours = before.replace("base ", "ours ")
        theirs = before.replace("base ", "theirs ")
        merged, count = merge_text(self.root, before.encode(), ours.encode(), theirs.encode())
        self.assertEqual(count, 130)
        self.assertIn(b"<<<<<<<< fork\n", merged)

    def test_preview_reports_executable_mode_changes(self):
        self.write("script.sh", "echo before\n")
        self.commit("script base")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("update-index", "--chmod=+x", "script.sh")
        self.git("commit", "-qm", "executable upstream")
        target = self.git("rev-parse", "HEAD").strip()
        report = json.loads(self.preview(base, target, "--fork-base", base).stdout)
        row = report["files"][0]
        self.assertEqual(row["status"], "clean")
        self.assertEqual(row["merged_mode"], "100755")

    def test_prepare_includes_preview_and_correct_module_imports(self):
        base, target = self.history("old\n", "github.com/EvilFreelancer/coddy-agent/internal/config\n", "old\n")
        tree = self.root / "worktrees" / "prepared"
        report = json.loads(self.run_script("prepare", "--wave", "preview", "--from", base, "--to", target,
                            "--worktree", str(tree), "--format", "json").stdout)
        bundle = Path(report["bundle"])
        preview = json.loads((bundle / "preview" / "preview.json").read_text())
        self.assertEqual(preview["files"][0]["status"], "clean")
        patch = (bundle / "adapted.patch").read_text()
        self.assertIn("github.com/hijera/foxxycode-agent/internal/config", patch)
        self.assertNotIn("github.com/EvilFreelancer/foxxycode-agent", patch)

    def test_preview_reports_file_directory_conflicts_without_merge_candidates(self):
        self.write("parent/child.txt", "upstream addition\n")
        self.write("leaf.txt", "upstream file\n")
        self.commit("upstream additions")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", self.base)
        self.write("parent", "fork file blocks directory\n")
        self.write("leaf.txt/child.txt", "fork child blocks file\n")
        self.commit("fork blockers")
        bundle = self.root / "structural"
        report = json.loads(self.preview(self.base, target, "--bundle", str(bundle)).stdout)
        rows = {row["path"]: row for row in report["files"]}
        for name, blockers in (("parent/child.txt", ["parent"]),
                               ("leaf.txt", ["leaf.txt/child.txt"])):
            with self.subTest(path=name):
                row = rows[name]
                self.assertEqual(row["status"], "path_conflict")
                self.assertEqual(row["blocking_paths"], blockers)
                self.assertNotIn("merged", row["artifacts"])
                self.assertNotIn("merged_mode", row)
        self.assertEqual(report["counts"], {"path_conflict": 2})
        self.assertIn('Blocked by: "parent"', (bundle / "preview.md").read_text())
        expected = {bundle / "preview.json", bundle / "preview.md"}
        expected.update(bundle / name for row in rows.values() for name in row["artifacts"].values())
        self.assertEqual({path for path in bundle.rglob("*") if path.is_file()}, expected)

    def test_preview_allows_clean_file_directory_transitions_but_keeps_modified_blockers(self):
        self.write("entry", "original\n")
        self.commit("upstream file")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("rm", "entry")
        self.write("entry/child.txt", "different content\n")
        self.commit("file becomes directory")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        clean = json.loads(self.preview(base, target).stdout)
        self.assertEqual(clean["counts"], {"add_clean": 1, "delete_clean": 1})
        self.write("entry", "fork modification\n")
        self.commit("modified blocker")
        report = json.loads(self.preview(base, target).stdout)
        self.assertEqual(report["counts"], {"delete_modify": 1, "path_conflict": 1})

    def test_preview_rejects_branding_file_directory_collisions_before_writing(self):
        self.write("coddy", "upstream file\n")
        self.write("foxxycode/child.txt", "upstream child\n")
        self.commit("branding prefix collision")
        bundle = self.root / "not-created"
        result = self.preview(self.base, "HEAD", "--bundle", str(bundle), expected=2)
        self.assertIn("collision", result.stderr.lower())
        self.assertFalse(bundle.exists())
        worktree = self.root / "worktrees" / "not-created"
        for flags in ((), ("--dry-run",)):
            with self.subTest(flags=flags):
                self.run_script("prepare", "--wave", "prefix", "--from", self.base, "--to", "HEAD",
                                "--worktree", str(worktree), *flags, expected=2)
                self.assertFalse(worktree.exists())
                self.assertNotIn("codex/upstream-prefix", self.git("branch", "--list"))

    def test_preview_allows_directory_to_file_transition_unless_fork_added_children(self):
        self.write("entry/old.txt", "old child\n")
        self.commit("upstream directory")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("rm", "entry/old.txt")
        self.write("entry", "new parent file\n")
        self.commit("directory becomes file")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        clean = json.loads(self.preview(base, target).stdout)
        self.assertEqual(clean["counts"], {"add_clean": 1, "delete_clean": 1})
        self.write("entry/local.txt", "fork-only child\n")
        self.commit("fork child")
        report = json.loads(self.preview(base, target).stdout)
        row = next(row for row in report["files"] if row["path"] == "entry")
        self.assertEqual(row["status"], "path_conflict")
        self.assertEqual(row["blocking_paths"], ["entry/local.txt"])

    def test_preview_allows_rename_into_source_directory(self):
        self.write("entry", "original\n")
        self.commit("upstream file")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("mv", "entry", "temporary")
        (self.root / "entry").mkdir()
        self.git("mv", "temporary", "entry/child.txt")
        self.commit("rename into directory")
        target = self.git("rev-parse", "HEAD").strip()
        report = json.loads(self.preview(base, target, "--fork-base", base).stdout)
        row, = report["files"]
        self.assertEqual(row["old_path"], "entry")
        self.assertEqual(row["status"], "clean")

    def test_preview_keeps_source_of_blocked_rename_as_a_blocker(self):
        self.write("old", "original\n")
        self.commit("upstream file")
        base = self.git("rev-parse", "HEAD").strip()
        self.git("mv", "old", "new")
        self.write("old/child.txt", "unrelated addition\n")
        self.commit("rename and reuse source directory")
        target = self.git("rev-parse", "HEAD").strip()
        self.git("checkout", "-q", "--detach", base)
        self.write("new/local.txt", "fork owns destination directory\n")
        self.commit("fork blocker")
        report = json.loads(self.preview(base, target).stdout)
        rows = {row["path"]: row for row in report["files"]}
        self.assertEqual(rows["new"]["status"], "path_conflict")
        self.assertEqual(rows["new"]["blocking_paths"], ["new/local.txt"])
        self.assertEqual(rows["old/child.txt"]["status"], "path_conflict")
        self.assertEqual(rows["old/child.txt"]["blocking_paths"], ["old"])

    def test_preview_refuses_common_and_private_metadata_from_linked_worktree(self):
        worktree = self.root / "linked"
        self.git("worktree", "add", "--detach", str(worktree), self.base)
        common = self.root / ".git"
        private = Path(self.git("-C", str(worktree), "rev-parse", "--absolute-git-dir").strip())
        original_root = self.root
        try:
            self.root = worktree
            for directory in (common, private):
                with self.subTest(directory=directory):
                    bundle = directory / "forbidden-preview"
                    result = self.preview(self.base, self.base, "--bundle", str(bundle), expected=2)
                    self.assertIn("Git metadata", result.stderr)
                    self.assertFalse(bundle.exists())
        finally:
            self.root = original_root
