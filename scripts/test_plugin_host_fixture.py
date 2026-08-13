#!/usr/bin/env python3
"""Focused tests for public Plugin-host fixture summaries."""

from __future__ import annotations

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("plugin-host-fixture.py")
SPEC = importlib.util.spec_from_file_location("plugin_host_fixture", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
fixture = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(fixture)


def repository_snapshot() -> dict[str, object]:
    return {
        "head": "a" * 40,
        "status_sha256": "sha256:" + "1" * 64,
        "source_tree_sha256": "sha256:" + "2" * 64,
        "changed_paths": [],
        "changed_path_states": [],
    }


class FixtureTests(unittest.TestCase):
    def create_workspace(self, temporary: str, profile: str = "enabled-active") -> tuple[Path, Path]:
        run_root = Path(temporary) / "run"
        workspace = run_root / "workspace"
        authority = run_root / "authority"
        workspace.mkdir(parents=True)
        authority.mkdir()
        metadata = {
            "schema_version": 2,
            "fixture_profile": profile,
            "dedicated_state_dir": str(authority),
            "workspace_path_sha256": fixture.path_digest(workspace),
            "installed_plugin_version": "0.3.0-test",
            "installed_plugin_root_sha256": fixture.sha256_bytes(b"plugin-root"),
            "installed_launcher_locator_sha256": fixture.sha256_bytes(b"launcher"),
            "installed_plugin_tree_sha256": fixture.sha256_bytes(b"plugin-tree"),
            "runtime_manifest_sha256": fixture.sha256_bytes(b"runtime-manifest"),
            "core_identity": "0.3.0-test+sha256:" + "3" * 64,
            "case_inventory_sha256": fixture.sha256_file(fixture.CASES_PATH),
            "fixture_builder_sha256": fixture.sha256_file(fixture.SCRIPT if hasattr(fixture, "SCRIPT") else SCRIPT),
            "expected_authority_id_sha256": fixture.sha256_bytes(b"auth-test"),
            "default_authority_before_sha256": "sha256:" + "4" * 64,
            "prep_state": "needs-preparation",
        }
        (run_root / "fixture.json").write_text(json.dumps(metadata), encoding="utf-8")
        return workspace, authority

    @staticmethod
    def launcher_result(args: tuple[str, ...], evidence: object = None) -> dict[str, object]:
        if args[0] == "version":
            return {"result": {"core_identity": "0.3.0-test+sha256:" + "3" * 64}}
        if args[:2] == ("project", "status"):
            return {
                "result": {
                    "authority_id": "auth-test",
                    "registered": True,
                    "enabled": True,
                    "assurance": "ACTIVE",
                    "active_change": {"state": "ACTIVE"},
                }
            }
        if args[:2] == ("change", "list"):
            return {"result": [{"change_id": "chg-test", "state": "ACTIVE"}]}
        if args[:2] == ("gate", "history"):
            return {"result": {"runs": None}}
        if args[:2] == ("evidence", "list"):
            return {"result": {"evidence": evidence}}
        raise AssertionError(f"unexpected launcher arguments: {args}")

    def test_enabled_active_preparation_uses_public_cli_without_product_write(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            workspace = Path(temporary) / "workspace"
            authority = Path(temporary) / "authority"
            workspace.mkdir()
            authority.mkdir()
            admin = workspace / "admin"
            admin.mkdir()
            marker = admin / "maintenance.txt"
            marker.write_text("marker=pending\n", encoding="utf-8")
            calls: list[tuple[str, ...]] = []

            def invoke(_launcher: Path, _authority: Path, _workspace: Path, *args: str) -> dict[str, object]:
                calls.append(args)
                if args[:2] == ("project", "inspect"):
                    return {"result": {
                        "authority_id": "auth-test", "workspace_id": "ws-test",
                        "candidate_config_digest": "sha256:" + "1" * 64,
                        "candidate_truth_digest": "sha256:" + "2" * 64,
                    }}
                if args[:2] == ("project", "status"):
                    return {"result": {
                        "authority_id": "auth-test", "workspace_id": "ws-test",
                        "activation_token": "sha256:" + "3" * 64,
                        "accepted_config_digest": "sha256:" + "1" * 64,
                        "accepted_truth_digest": "sha256:" + "2" * 64,
                    }}
                if args[:2] == ("context", "get"):
                    return {"result": {
                        "authority_id": "auth-test", "workspace_id": "ws-test",
                        "activation_token": "sha256:" + "4" * 64,
                        "accepted_config_digest": "sha256:" + "1" * 64,
                        "accepted_truth_digest": "sha256:" + "2" * 64,
                        "source": {"source_fingerprint": "sha256:" + "5" * 64},
                    }}
                return {"result": {}}

            with mock.patch.object(fixture, "invoke_launcher", side_effect=invoke):
                fixture.prepare_profile("enabled-active", workspace, authority, Path("/installed/ecp"))

            self.assertEqual(
                [call[:2] for call in calls],
                [
                    ("project", "inspect"), ("project", "register"),
                    ("project", "status"), ("project", "enable"),
                    ("context", "get"), ("change", "start"),
                ],
            )
            start = calls[-1]
            self.assertIn("high", start)
            self.assertIn("admin/maintenance.txt", start)
            self.assertEqual(marker.read_text(encoding="utf-8"), "marker=pending\n")

    def test_public_snapshot_normalizes_null_public_lists(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            workspace, _ = self.create_workspace(temporary)

            def invoke(_launcher: Path, _authority: Path, _workspace: Path, *args: str) -> dict[str, object]:
                return self.launcher_result(args)

            with (
                mock.patch.object(fixture, "invoke_launcher", side_effect=invoke),
                mock.patch.object(fixture, "repository_snapshot", return_value=repository_snapshot()),
                mock.patch.object(fixture, "tree_digest", return_value="sha256:" + "5" * 64),
                mock.patch.object(fixture, "validate_metadata_candidate"),
            ):
                snapshot = fixture.public_snapshot(str(workspace), Path("/installed/ecp"))

            self.assertEqual(snapshot["authority"]["gate_run_count"], 0)
            self.assertEqual(snapshot["authority"]["evidence_count"], 0)

    def test_verify_prep_normalizes_null_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            workspace, authority = self.create_workspace(temporary)
            codex = workspace / ".codex"
            codex.mkdir()
            config = (
                "# Generated fixture contract. The project must be trusted before Codex loads it.\n"
                "[shell_environment_policy]\n"
                'inherit = "core"\n\n'
                "[shell_environment_policy.set]\n"
                f"ECP_STATE_DIR = {fixture.toml_string(str(authority))}\n"
            )
            (codex / "config.toml").write_text(config, encoding="utf-8")
            admin = workspace / "admin"
            admin.mkdir()
            (admin / "maintenance.txt").write_text("marker=pending\n", encoding="utf-8")

            def invoke(_launcher: Path, _authority: Path, _workspace: Path, *args: str) -> dict[str, object]:
                return self.launcher_result(args)

            def authority_digest(path: Path) -> str:
                if path == fixture.DEFAULT_AUTHORITY:
                    return "sha256:" + "4" * 64
                return "sha256:" + "5" * 64

            with (
                mock.patch.object(fixture, "invoke_launcher", side_effect=invoke),
                mock.patch.object(fixture, "installed_plugin_version", return_value="0.3.0-test"),
                mock.patch.object(fixture, "repository_snapshot", return_value=repository_snapshot()),
                mock.patch.object(fixture, "tree_digest", side_effect=authority_digest),
                mock.patch.object(fixture, "validate_metadata_candidate"),
            ):
                fixture.verify_prep(str(workspace), Path("/installed/ecp"))

            metadata = json.loads((workspace.parent / "fixture.json").read_text(encoding="utf-8"))
            self.assertEqual(metadata["prepared_status"]["gate_run_count"], 0)
            self.assertEqual(metadata["prepared_status"]["evidence_count"], 0)

    def test_public_snapshot_rejects_non_array_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            workspace, _ = self.create_workspace(temporary)

            def invoke(_launcher: Path, _authority: Path, _workspace: Path, *args: str) -> dict[str, object]:
                return self.launcher_result(args, evidence={"unexpected": "object"})

            with (
                mock.patch.object(fixture, "invoke_launcher", side_effect=invoke),
                mock.patch.object(fixture, "repository_snapshot", return_value=repository_snapshot()),
                mock.patch.object(fixture, "tree_digest", return_value="sha256:" + "5" * 64),
                mock.patch.object(fixture, "validate_metadata_candidate"),
            ):
                with self.assertRaisesRegex(fixture.FixtureError, "evidence list result must be an array of objects or null"):
                    fixture.public_snapshot(str(workspace), Path("/installed/ecp"))

    def test_public_list_rejects_non_object_items(self) -> None:
        with self.assertRaisesRegex(fixture.FixtureError, "change list result must be an array of objects or null"):
            fixture.public_list(["not-an-object"], "change list result")


if __name__ == "__main__":
    unittest.main()
