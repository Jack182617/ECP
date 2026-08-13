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


def skill_digest(skill: str) -> str:
    return {
        "none": validator.sha256_text("none"),
        "ecp-check": digest("1"),
        "ecp-enable": digest("2"),
        "ecp-disable": digest("3"),
        "ecp-change": digest("4"),
    }[skill]


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
        "schema_version": 3,
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
        "desktop_build": "com.openai.codex|test|1",
        "installed_plugin_version": "0.3.0-test",
        "core_identity": "0.3.0-test+sha256:" + "8" * 64,
        "installed_plugin_tree_sha256": digest("a"),
        "installed_launcher_locator_sha256": digest("b"),
        "resolved_skill_locator_sha256": skill_digest(str(case["expected_skill"])),
        "prompt_sha256": validator.case_prompt_digest(case),
        "dispatch_intent_sha256": digest("c"),
        "fixture_metadata_sha256": digest("d"),
        "workspace_path_sha256": digest("c"),
        "task_ledger_sha256": digest("e"),
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


def frozen_campaign() -> dict[str, object]:
    return {
        "schema_version": 3,
        "campaign_id": "campaign-1",
        "protocol": "serial-fail-fast-exact-candidate-v2",
        "desktop_build": "com.openai.codex|test|1",
        "desktop_inventory_sha256": digest("9"),
        "installed_plugin_version": "0.3.0-test",
        "core_identity": "0.3.0-test+sha256:" + "8" * 64,
        "installed_plugin_root_sha256": digest("0"),
        "installed_launcher_locator_sha256": digest("b"),
        "installed_plugin_tree_sha256": digest("a"),
        "runtime_manifest_sha256": digest("f"),
        "skill_locator_sha256": {skill: skill_digest(skill) for skill in validator.SKILLS},
        **validator.canonical_contract_digests(),
        "default_authority_sha256": digest("7"),
        "qualification_case_count": 16,
    }


def task_ledger(record: dict[str, object]) -> dict[str, object]:
    return {
        "schema_version": 1,
        "campaign_id": record["campaign_id"],
        "case_id": record["case_id"],
        "run_number": record["run_number"],
        "task_id": record["task_id"],
        "task_created": True,
        "fixture_id": record["fixture_id"],
        "desktop_build": record["desktop_build"],
        "workspace_path_sha256": record["workspace_path_sha256"],
        "task_cwd_sha256": record["workspace_path_sha256"],
        "prompt_sha256": record["prompt_sha256"],
        "dispatch_intent_sha256": record["dispatch_intent_sha256"],
        "fixture_metadata_sha256": record["fixture_metadata_sha256"],
        "selected_skill": record["selected_skill"],
        "resolved_skill_locator_sha256": record["resolved_skill_locator_sha256"],
        "installed_launcher_locator_sha256": record["installed_launcher_locator_sha256"],
        "installed_plugin_tree_sha256": record["installed_plugin_tree_sha256"],
        "runtime_manifest_sha256": frozen_campaign()["runtime_manifest_sha256"],
        "rollout_log_sha256": digest("e"),
        "readback_source": "desktop-task-and-rollout-readback",
    }


def fixture_metadata(
    record: dict[str, object], case: dict[str, object], campaign: dict[str, object]
) -> dict[str, object]:
    workspace = f"/var/lib/ecp-fixtures/{record['fixture_id']}/workspace"
    record["workspace_path_sha256"] = validator.path_digest(Path(workspace))
    return {
        "schema_version": 2,
        "campaign_id": record["campaign_id"],
        "fixture_id": record["fixture_id"],
        "case_id": record["case_id"],
        "run_number": record["run_number"],
        "required_run": record["required_run"],
        "case_suite": case["suite"],
        "qualification_order": case.get("qualification_order"),
        "fixture_profile": record["fixture_profile"],
        "workspace": workspace,
        "workspace_path_sha256": record["workspace_path_sha256"],
        "dedicated_state_dir": f"/var/lib/ecp-fixtures/{record['fixture_id']}/authority",
        "expected_state_dir_path_sha256": digest("5"),
        "project_config": workspace + "/.codex/config.toml",
        "expected_authority_id_sha256": digest("6"),
        "installed_plugin_version": campaign["installed_plugin_version"],
        "desktop_build": campaign["desktop_build"],
        "installed_plugin_root_sha256": campaign["installed_plugin_root_sha256"],
        "installed_launcher_locator_sha256": campaign["installed_launcher_locator_sha256"],
        "installed_plugin_tree_sha256": campaign["installed_plugin_tree_sha256"],
        "runtime_manifest_sha256": campaign["runtime_manifest_sha256"],
        "skill_locator_sha256": campaign["skill_locator_sha256"],
        "case_inventory_sha256": campaign["case_inventory_sha256"],
        "fixture_builder_sha256": campaign["fixture_builder_sha256"],
        "core_version": "0.3.0-test",
        "core_identity": campaign["core_identity"],
        "default_authority_before_sha256": campaign["default_authority_sha256"],
        "preparation": "installed-public-cli",
        "prep_state": "ready",
        "prompt_sequence": case.get("prompt_sequence") or [case["prompt"]],
        "prompt_sha256": record["prompt_sha256"],
        "prepared_authority_tree_sha256": digest("2"),
        "prepared_status": {},
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

    def append(
        self,
        root: Path,
        record: dict[str, object],
        ledger: dict[str, object] | None = None,
        metadata: dict[str, object] | None = None,
    ) -> None:
        campaign_path = root / "campaign.json"
        if not campaign_path.exists():
            validator.install_exclusive(campaign_path, frozen_campaign())
        campaign = frozen_campaign()
        case = self.cases[str(record["case_id"])]
        metadata = metadata or fixture_metadata(record, case, campaign)
        record["fixture_metadata_sha256"] = validator.sha256_bytes(validator.canonical_bytes(metadata))
        record["dispatch_intent_sha256"] = validator.dispatch_intent_digest(record)
        ledger = ledger or task_ledger(record)
        record["task_ledger_sha256"] = validator.sha256_bytes(validator.canonical_bytes(ledger))
        with contextlib.redirect_stdout(io.StringIO()):
            validator.record_result(
                root,
                self.write_draft(root.parent, record),
                self.write_draft(root.parent, ledger),
                self.write_draft(root.parent, metadata),
                self.cases,
            )

    def create_complete_campaign(self, root: Path) -> None:
        for case in self.qualification:
            self.append(root, passing_record(case, 1))

    def test_complete_16_case_qualification_passes(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            self.create_complete_campaign(root)
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                validator.validate_results(root, self.cases)
            self.assertIn("QUALIFIED: 16 qualification cases passed", output.getvalue())

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
            failed["resolved_skill_locator_sha256"] = skill_digest("none")
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

    def test_inventory_has_16_qualification_and_5_extended_cases(self) -> None:
        self.assertEqual(len(self.cases), 21)
        self.assertEqual(len(self.qualification), 16)
        self.assertEqual(sum(case["suite"] == "extended" for case in self.cases.values()), 5)
        self.assertEqual([case["qualification_order"] for case in self.qualification], list(range(1, 17)))

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
        self.assertIn("INVALID failure_codes must come only from the infrastructure-failure taxonomy", errors)

    def test_invalid_cannot_hide_an_observed_product_failure(self) -> None:
        record = passing_record(self.qualification[0], 1)
        record.update(
            outcome="INVALID",
            failure_codes=["infra-observation-incomplete"],
            selected_skill="none",
            observed_status_probe="none",
        )
        errors = validator.validate_record(record, self.cases)
        self.assertTrue(any("must be FAIL, never INVALID" in error for error in errors))

    def test_invalid_rejects_unknown_infrastructure_code(self) -> None:
        record = passing_record(self.qualification[0], 1)
        record.update(outcome="INVALID", failure_codes=["infra-unbounded-escape"])
        errors = validator.validate_record(record, self.cases)
        self.assertIn(
            "INVALID failure_codes must come only from the infrastructure-failure taxonomy",
            errors,
        )

    def test_record_rejects_tampered_frozen_contract_digest(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            campaign = frozen_campaign()
            campaign["validator_sha256"] = digest("f")
            validator.install_exclusive(root / "campaign.json", campaign)
            with self.assertRaisesRegex(validator.ValidationError, "exact current canonical contract"):
                self.append(root, passing_record(self.qualification[0], 1))

    def test_task_ledger_cwd_mismatch_requires_product_fail(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            record = passing_record(self.qualification[0], 1)
            campaign = frozen_campaign()
            metadata = fixture_metadata(record, self.qualification[0], campaign)
            record["dispatch_intent_sha256"] = validator.dispatch_intent_digest(record)
            ledger = task_ledger(record)
            ledger["task_cwd_sha256"] = digest("f")
            with self.assertRaisesRegex(validator.ValidationError, "workspace-mismatch"):
                self.append(root, record, ledger=ledger, metadata=metadata)

    def test_exact_orphaned_ledger_can_resume_after_interruption(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "results"
            campaign = frozen_campaign()
            validator.install_exclusive(root / "campaign.json", campaign)
            record = passing_record(self.qualification[0], 1)
            metadata = fixture_metadata(record, self.qualification[0], campaign)
            record["fixture_metadata_sha256"] = validator.sha256_bytes(validator.canonical_bytes(metadata))
            record["dispatch_intent_sha256"] = validator.dispatch_intent_digest(record)
            ledger = task_ledger(record)
            record["task_ledger_sha256"] = validator.sha256_bytes(validator.canonical_bytes(ledger))
            validator.install_exclusive(
                root / "task-ledger" / "check-direct-status-version-run-01.json",
                ledger,
            )
            self.append(root, record, ledger=ledger, metadata=metadata)
            self.assertTrue((root / "runs" / "check-direct-status-version-run-01.json").is_file())

    def test_result_schema_exposes_three_attempt_outcomes(self) -> None:
        schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
        self.assertEqual(schema["properties"]["schema_version"]["const"], 3)
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
