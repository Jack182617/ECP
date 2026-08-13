#!/usr/bin/env python3
"""Focused fail-closed tests for Plugin host qualification results."""

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
SCHEMA = SCRIPT.parents[1] / "docs" / "plugin-host-evaluation-result.schema.json"
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
        "schema_version": 2,
        "campaign_id": "campaign-1",
        "case_id": case["id"],
        "run_number": run_number,
        "required_run": case["suite"] == "qualification" and run_number == 1,
        "task_id": f"task-{case['id']}-{run_number}",
        "fixture_id": f"{case['id']}-run-{run_number:02d}",
        "fixture_profile": profile,
        "initial_project_mode": case["initial_mode"],
        "environment_probe": {
            "project_trusted": True,
            "project_config_loaded": True,
            "installed_launcher_only": True,
            "fixture_state_dir_path_sha256": digest("5"),
            "operator_state_dir_path_sha256": digest("5"),
            "fixture_authority_id_sha256": digest("6"),
            "operator_authority_id_sha256": digest("6"),
            "fixture_plugin_version_sha256": validator.sha256_text("0.3.0-test"),
            "operator_plugin_version_sha256": validator.sha256_text("0.3.0-test"),
            "fixture_core_identity_sha256": validator.sha256_text("0.3.0-test+sha256:" + "8" * 64),
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
        cls.qualification = validator.qualification_cases(cls.cases)

    @staticmethod
    def write_draft(parent: Path, record: dict[str, object]) -> Path:
        drafts = parent / "drafts"
        drafts.mkdir(exist_ok=True)
        path = drafts / f"{record['case_id']}-{record['run_number']}-{len(list(drafts.iterdir()))}.json"
        path.write_text(json.dumps(record), encoding="utf-8")
        return path

    def append(self, root: Path, record: dict[str, object]) -> None:
        with contextlib.redirect_stdout(io.StringIO()):
            validator.record_result(root, self.write_draft(root.parent, record), self.cases)

    def create_complete_campaign(self, root: Path) -> None:
        for case in self.qualification:
            self.append(root, passing_record(case, 1))

    def test_complete_12_case_qualification_passes(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            self.create_complete_campaign(root)
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                validator.validate_results(root, self.cases)
            self.assertIn("QUALIFIED: 12 qualification cases passed", output.getvalue())

    def test_invalid_attempt_can_retry_in_fresh_fixture(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            first = passing_record(self.qualification[0], 1)
            first.update(outcome="INVALID", failure_codes=["infra-dispatch-timeout"])
            first["environment_probe"]["project_config_loaded"] = False
            self.append(root, first)
            self.append(root, passing_record(self.qualification[0], 2))
            for case in self.qualification[1:]:
                self.append(root, passing_record(case, 1))
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                validator.validate_results(root, self.cases)
            self.assertIn("1 INVALID attempts", output.getvalue())

    def test_second_invalid_is_terminal_for_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            for run_number in (1, 2):
                invalid = passing_record(self.qualification[0], run_number)
                invalid.update(outcome="INVALID", failure_codes=["infra-dispatch-timeout"])
                invalid["environment_probe"]["project_config_loaded"] = False
                self.append(root, invalid)
            with self.assertRaisesRegex(validator.ValidationError, "one fresh-fixture retry also failed"):
                self.append(root, passing_record(self.qualification[0], 3))
            with self.assertRaisesRegex(validator.ValidationError, "infrastructure failed twice"):
                validator.validate_results(root, self.cases)

    def test_product_fail_is_terminal(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            failed = passing_record(self.qualification[0], 1)
            failed.update(outcome="FAIL", failure_codes=["wrong-route"], selected_skill="none")
            self.append(root, failed)
            with self.assertRaisesRegex(validator.ValidationError, "product FAIL is terminal"):
                self.append(root, passing_record(self.qualification[1], 1))
            with self.assertRaisesRegex(validator.ValidationError, "FAILED: product behavior failed"):
                validator.validate_results(root, self.cases)

    def test_qualification_is_serial(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            with self.assertRaisesRegex(validator.ValidationError, "next case must be"):
                self.append(root, passing_record(self.qualification[1], 1))

    def test_missing_case_is_incomplete(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            self.append(root, passing_record(self.qualification[0], 1))
            with self.assertRaisesRegex(validator.ValidationError, "INCOMPLETE"):
                validator.validate_results(root, self.cases)

    def test_inventory_has_12_qualification_and_9_extended_cases(self) -> None:
        self.assertEqual(len(self.cases), 21)
        self.assertEqual(len(self.qualification), 12)
        self.assertEqual(sum(case["suite"] == "extended" for case in self.cases.values()), 9)
        self.assertEqual([case["qualification_order"] for case in self.qualification], list(range(1, 13)))

    def test_extended_case_requires_completed_qualification(self) -> None:
        extended = next(case for case in self.cases.values() if case["suite"] == "extended")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            with self.assertRaisesRegex(validator.ValidationError, "only after qualification is QUALIFIED"):
                self.append(root, passing_record(extended, 1))

    def test_established_enablement_preserves_existing_dirty_content(self) -> None:
        case = self.cases["enable-direct-established"]
        record = passing_record(case, 1)
        record["after"]["repository"]["changed_path_states"][0]["sha256"] = digest("9")
        errors = validator.validate_record(record, self.cases)
        self.assertTrue(any("pre-existing repository change was not preserved" in error for error in errors))

    def test_operator_package_identity_must_match_fixture(self) -> None:
        record = passing_record(self.qualification[0], 1)
        record["environment_probe"]["operator_core_identity_sha256"] = digest("9")
        errors = validator.validate_record(record, self.cases)
        self.assertTrue(any("different Core identities" in error for error in errors))

    def test_invalid_requires_infrastructure_failure_code(self) -> None:
        record = passing_record(self.qualification[0], 1)
        record.update(outcome="INVALID", failure_codes=["wrong-route"])
        errors = validator.validate_record(record, self.cases)
        self.assertIn("INVALID must include an infra- failure code", errors)

    def test_result_schema_exposes_three_attempt_outcomes(self) -> None:
        schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
        self.assertEqual(schema["properties"]["schema_version"]["const"], 2)
        self.assertEqual(schema["properties"]["outcome"]["enum"], ["PASS", "FAIL", "INVALID"])

    def test_results_root_rejects_source_temp_authority_and_cache(self) -> None:
        with self.assertRaisesRegex(validator.ValidationError, "absolute"):
            validator.results_root("relative/results")
        for path in (
            validator.REPO_ROOT / "results",
            Path(tempfile.gettempdir()) / "ecp-results",
            Path("/var/lib/authority/ecp-results"),
            Path("/var/lib/cache/ecp-results"),
        ):
            with self.subTest(path=path):
                with self.assertRaises(validator.ValidationError):
                    validator.results_root(str(path))
        self.assertEqual(
            validator.results_root("/var/lib/ecp-plugin-results/campaign-1"),
            Path("/var/lib/ecp-plugin-results/campaign-1").resolve(),
        )


if __name__ == "__main__":
    unittest.main()
