#!/usr/bin/env python3
"""Focused fail-closed tests for the canonical Plugin host result validator."""

from __future__ import annotations

import copy
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("validate-plugin-host-results.py")
SPEC = importlib.util.spec_from_file_location("plugin_host_results", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
validator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validator)


def digest(character: str) -> str:
    return "sha256:" + character * 64


def repository(
    changed_paths: list[str], marker: str, preserved: dict[str, str] | None = None
) -> dict[str, object]:
    preserved = preserved or {}
    return {
        "head": "a" * 40,
        "status_sha256": digest(marker),
        "source_tree_sha256": digest(marker),
        "changed_paths": sorted(changed_paths),
        "changed_path_states": [
            {"path": path, "sha256": preserved.get(path, digest(marker))}
            for path in sorted(changed_paths)
        ],
    }


def authority(profile: str, marker: str) -> dict[str, object]:
    result: dict[str, object] = {
        "tree_sha256": digest(marker),
        "registered": False,
        "enabled": False,
        "assurance": "DISABLED",
        "active_change_count": 0,
        "active_gate_run_count": 0,
        "change_count": 0,
        "completed_change_count": 0,
        "cancelled_change_count": 0,
        "gate_run_count": 0,
        "evidence_count": 0,
    }
    if profile == "enabled-clean":
        result.update(registered=True, enabled=True, assurance="READY")
    elif profile == "enabled-active":
        result.update(registered=True, enabled=True, assurance="ACTIVE", active_change_count=1, change_count=1)
    elif profile == "enabled-blocked":
        result.update(registered=True, enabled=True, assurance="BLOCKED")
    return result


def passing_record(case: dict[str, object], run_number: int) -> dict[str, object]:
    profile = str(case["fixture_profile"])
    before_paths: list[str] = []
    if profile == "established-disabled":
        before_paths = ["notes/operator-note.txt"]
    elif profile == "enabled-blocked":
        before_paths = [".ecp/policy.json"]
    before_repository = repository(before_paths, "1")
    after_repository = copy.deepcopy(before_repository)
    before_authority = authority(profile, "2")
    after_authority = copy.deepcopy(before_authority)

    expected_repository = str(case["repository_mutation"])
    if expected_repository == "enablement-only" and profile == "greenfield-disabled":
        after_repository = repository([".ecp/project.json"], "3")
        observed_repository = "enablement-only"
    elif expected_repository == "enablement-only":
        observed_repository = "none"
    elif expected_repository == "governed-only":
        after_repository = repository(["src/session.py", "tests/test_session.py"], "3")
        observed_repository = "governed-only"
    elif expected_repository == "ordinary-after-disabled-status":
        after_repository = repository(["src/session.py", "tests/test_session.py"], "3")
        observed_repository = "ordinary-after-disabled-status"
    else:
        observed_repository = "none"

    expected_authority = str(case["authority_mutation"])
    observed_authority = "none" if expected_authority == "forbidden" else expected_authority
    if expected_authority == "enablement":
        after_authority.update(tree_sha256=digest("4"), registered=True, enabled=True, assurance="READY")
    elif expected_authority in {"disablement", "cancel-and-disable"}:
        after_authority.update(tree_sha256=digest("4"), registered=True, enabled=False, assurance="DISABLED", active_change_count=0)
        if expected_authority == "cancel-and-disable":
            after_authority["cancelled_change_count"] = int(before_authority["cancelled_change_count"]) + 1
    elif expected_authority == "change-cancellation":
        after_authority.update(tree_sha256=digest("4"), registered=True, enabled=True, assurance="READY", active_change_count=0)
        after_authority["cancelled_change_count"] = int(before_authority["cancelled_change_count"]) + 1
    elif expected_authority == "governed-completion":
        after_authority.update(
            tree_sha256=digest("4"), registered=True, enabled=True, assurance="READY",
            active_change_count=0, change_count=int(before_authority["change_count"]) + 1,
            completed_change_count=int(before_authority["completed_change_count"]) + 1,
            gate_run_count=int(before_authority["gate_run_count"]) + 1,
            evidence_count=int(before_authority["evidence_count"]) + 1,
        )

    expected_mode = str(case["project_mode_mutation"])
    observed_mode = "none" if expected_mode == "forbidden" else expected_mode
    status_probe = "none" if case["status_probe"] == "forbidden" else "installed-launcher-before-first-mutation"
    final_turn = {
        "turn_number": len(case.get("prompt_sequence") or [case["prompt"]]),
        "selected_skill": case["expected_skill"],
        "observed_status_probe": status_probe,
        "observed_repository_mutation": observed_repository,
        "observed_authority_mutation": observed_authority,
        "observed_project_mode_mutation": observed_mode,
    }
    turn_observations = [final_turn]
    if case.get("prompt_sequence"):
        turn_observations = [{
            "turn_number": 1,
            "selected_skill": "none",
            "observed_status_probe": "none",
            "observed_repository_mutation": "none",
            "observed_authority_mutation": "none",
            "observed_project_mode_mutation": "none",
        }, final_turn]
    return {
        "schema_version": 1,
        "campaign_id": "campaign-1",
        "case_id": case["id"],
        "run_number": run_number,
        "required_run": run_number <= int(case["minimum_runs"]),
        "task_id": f"task-{case['id']}-{run_number}",
        "fixture_id": f"{case['id']}-run-{run_number:02d}",
        "fixture_profile": profile,
        "initial_project_mode": case["initial_mode"],
        "environment_probe": {
            "project_trusted": True,
            "project_config_loaded": True,
            "installed_launcher_only": True,
            "task_authority_matches_operator": True,
            "fixture_state_dir_path_sha256": digest("5"),
            "task_state_dir_path_sha256": digest("5"),
            "operator_state_dir_path_sha256": digest("5"),
            "fixture_authority_id_sha256": digest("6"),
            "task_authority_id_sha256": digest("6"),
            "operator_authority_id_sha256": digest("6"),
            "fixture_plugin_version_sha256": validator.sha256_text("0.3.0-test"),
            "task_plugin_version_sha256": validator.sha256_text("0.3.0-test"),
            "operator_plugin_version_sha256": validator.sha256_text("0.3.0-test"),
            "fixture_core_identity_sha256": validator.sha256_text("0.3.0-test+sha256:" + "8" * 64),
            "task_core_identity_sha256": validator.sha256_text("0.3.0-test+sha256:" + "8" * 64),
            "operator_core_identity_sha256": validator.sha256_text("0.3.0-test+sha256:" + "8" * 64),
            "default_authority_before_sha256": digest("7"),
            "default_authority_after_sha256": digest("7"),
            "default_authority_unchanged": True,
        },
        "installed_plugin_version": "0.3.0-test",
        "core_identity": "0.3.0-test+sha256:" + "8" * 64,
        "turn_observations": turn_observations,
        "selected_skill": case["expected_skill"],
        "observed_status_probe": status_probe,
        "observed_repository_mutation": observed_repository,
        "observed_authority_mutation": observed_authority,
        "observed_project_mode_mutation": observed_mode,
        "expected_behavior_conformant": True,
        "prohibitions_preserved": True,
        "before": {"repository": before_repository, "authority": before_authority},
        "after": {"repository": after_repository, "authority": after_authority},
        "outcome": "PASS",
        "failure_codes": [],
        "notes": "",
    }


class ValidatorTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.cases = validator.load_cases(validator.DEFAULT_CASES)

    def create_complete_campaign(self, root: Path) -> dict[tuple[str, int], dict[str, object]]:
        records: dict[tuple[str, int], dict[str, object]] = {}
        for case_id, case in self.cases.items():
            for run_number in range(1, int(case["minimum_runs"]) + 1):
                record = passing_record(case, run_number)
                with contextlib.redirect_stdout(io.StringIO()):
                    validator.record_result(root, self.write_draft(root.parent, record), self.cases)
                records[(case_id, run_number)] = record
        return records

    def write_draft(self, parent: Path, record: dict[str, object]) -> Path:
        drafts = parent / "drafts"
        drafts.mkdir(exist_ok=True)
        path = drafts / f"{record['case_id']}-{record['run_number']}-{len(list(drafts.iterdir()))}.json"
        path.write_text(json.dumps(record), encoding="utf-8")
        return path

    def test_complete_42_run_matrix_passes(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            self.create_complete_campaign(root)
            validator.validate_results(root, self.cases)

    def test_missing_required_run_fails(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            self.create_complete_campaign(root)
            next((root / "runs").iterdir()).unlink()
            with self.assertRaisesRegex(validator.ValidationError, "missing required runs"):
                validator.validate_results(root, self.cases)

    def test_inventory_with_one_case_removed_fails(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            inventory = validator.load_json(validator.DEFAULT_CASES)
            inventory["cases"].pop()
            path = Path(temporary) / "cases.json"
            path.write_text(json.dumps(inventory), encoding="utf-8")
            with self.assertRaisesRegex(validator.ValidationError, "exactly 21 cases"):
                validator.load_cases(path)

    def test_duplicate_logical_run_fails(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            self.create_complete_campaign(root)
            source = next((root / "runs").iterdir())
            (root / "runs" / "duplicate.json").write_bytes(source.read_bytes())
            with self.assertRaisesRegex(validator.ValidationError, "duplicate logical run"):
                validator.validate_results(root, self.cases)

    def test_failed_required_run_cannot_be_overwritten(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            case = next(iter(self.cases.values()))
            failed = passing_record(case, 1)
            failed.update(outcome="FAIL", failure_codes=["wrong-route"], selected_skill="none")
            with contextlib.redirect_stdout(io.StringIO()):
                validator.record_result(root, self.write_draft(Path(temporary), failed), self.cases)
            replacement = passing_record(case, 1)
            with self.assertRaisesRegex(validator.ValidationError, "refusing to overwrite"):
                validator.record_result(root, self.write_draft(Path(temporary), replacement), self.cases)

    def test_default_authority_change_breaks_campaign_identity(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            cases = list(self.cases.values())
            first = passing_record(cases[0], 1)
            with contextlib.redirect_stdout(io.StringIO()):
                validator.record_result(root, self.write_draft(Path(temporary), first), self.cases)
            second = passing_record(cases[1], 1)
            second["environment_probe"]["default_authority_before_sha256"] = digest("9")
            second["environment_probe"]["default_authority_after_sha256"] = digest("9")
            with self.assertRaisesRegex(validator.ValidationError, "differs from campaign"):
                validator.record_result(root, self.write_draft(Path(temporary), second), self.cases)

    def test_unauthorized_authority_mutation_cannot_pass(self) -> None:
        case = next(iter(self.cases.values()))
        record = passing_record(case, 1)
        record["observed_authority_mutation"] = "unauthorized"
        self.assertTrue(any("observed_authority_mutation" in error for error in validator.validate_record(record, self.cases)))

    def test_follow_up_first_turn_cannot_prime_ecp(self) -> None:
        case = self.cases["change-indirect-follow-up-implementation"]
        record = passing_record(case, 1)
        record["turn_observations"][0]["selected_skill"] = "ecp-change"
        record["turn_observations"][0]["observed_status_probe"] = "installed-launcher-before-first-mutation"
        errors = validator.validate_record(record, self.cases)
        self.assertTrue(any("read-only pre-final prompt turn" in error for error in errors))

    def test_established_enablement_must_preserve_existing_dirty_content(self) -> None:
        case = self.cases["enable-direct-established"]
        record = passing_record(case, 1)
        record["after"]["repository"]["changed_path_states"][0]["sha256"] = digest("9")
        errors = validator.validate_record(record, self.cases)
        self.assertTrue(any("pre-existing repository change was not preserved" in error for error in errors))

    def test_task_and_operator_package_identity_must_match_fixture(self) -> None:
        case = next(iter(self.cases.values()))
        record = passing_record(case, 1)
        record["environment_probe"]["task_core_identity_sha256"] = digest("9")
        errors = validator.validate_record(record, self.cases)
        self.assertTrue(any("different Core identities" in error for error in errors))

    def test_cancelled_change_cannot_masquerade_as_governed_completion(self) -> None:
        case = self.cases["change-direct-enabled-edit"]
        record = passing_record(case, 1)
        authority_after = record["after"]["authority"]
        authority_after["completed_change_count"] = 0
        authority_after["cancelled_change_count"] = 1
        errors = validator.validate_record(record, self.cases)
        self.assertTrue(any("COMPLETED Change" in error for error in errors))

    def test_results_root_must_be_absolute_and_outside_source(self) -> None:
        with self.assertRaises(validator.ValidationError):
            validator.results_root("relative/results")
        with self.assertRaises(validator.ValidationError):
            validator.results_root(str(validator.REPO_ROOT / "results"))


if __name__ == "__main__":
    unittest.main()
