#!/usr/bin/env python3
"""Record and validate the append-only 42-run Plugin host matrix."""

from __future__ import annotations

import argparse
import errno
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import tempfile
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CASES = REPO_ROOT / "docs" / "plugin-host-evaluation-cases.json"
SHA256 = re.compile(r"^sha256:[0-9a-f]{64}$")
IDENTIFIER = re.compile(r"^[a-z0-9][a-z0-9._-]*$")
TOP_KEYS = {
    "schema_version", "campaign_id", "case_id", "run_number", "required_run",
    "task_id", "fixture_id", "fixture_profile", "initial_project_mode",
    "environment_probe", "installed_plugin_version", "core_identity",
    "turn_observations", "selected_skill", "observed_status_probe", "observed_repository_mutation",
    "observed_authority_mutation", "observed_project_mode_mutation",
    "expected_behavior_conformant", "prohibitions_preserved", "before", "after",
    "outcome", "failure_codes", "notes",
}
ENV_KEYS = {
    "project_trusted", "project_config_loaded", "installed_launcher_only",
    "task_authority_matches_operator", "fixture_state_dir_path_sha256", "task_state_dir_path_sha256",
    "operator_state_dir_path_sha256",
    "fixture_authority_id_sha256", "task_authority_id_sha256", "operator_authority_id_sha256",
    "fixture_plugin_version_sha256", "task_plugin_version_sha256", "operator_plugin_version_sha256",
    "fixture_core_identity_sha256", "task_core_identity_sha256", "operator_core_identity_sha256",
    "default_authority_before_sha256", "default_authority_after_sha256",
    "default_authority_unchanged",
}
REPOSITORY_KEYS = {"head", "status_sha256", "source_tree_sha256", "changed_paths", "changed_path_states"}
TURN_KEYS = {
    "turn_number", "selected_skill", "observed_status_probe",
    "observed_repository_mutation", "observed_authority_mutation",
    "observed_project_mode_mutation",
}
AUTHORITY_KEYS = {
    "tree_sha256", "registered", "enabled", "assurance", "active_change_count",
    "active_gate_run_count", "change_count", "completed_change_count",
    "cancelled_change_count", "gate_run_count", "evidence_count",
}
SKILLS = {"none", "ecp-check", "ecp-enable", "ecp-disable", "ecp-change"}
PROFILES = {
    "greenfield-disabled", "established-disabled", "enabled-clean",
    "enabled-active", "enabled-blocked",
}
ASSURANCES = {"DISABLED", "READY", "ACTIVE", "BLOCKED", "INDETERMINATE"}


class ValidationError(RuntimeError):
    pass


def load_json(path: Path) -> Any:
    try:
        with path.open(encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, json.JSONDecodeError) as error:
        raise ValidationError(f"{path}: cannot read canonical JSON: {error}") from error


def load_cases(path: Path) -> dict[str, dict[str, Any]]:
    inventory = load_json(path)
    if not isinstance(inventory, dict) or inventory.get("schema_version") != 2:
        raise ValidationError("case inventory must use schema_version 2")
    cases: dict[str, dict[str, Any]] = {}
    for case in inventory.get("cases", []):
        if not isinstance(case, dict) or not isinstance(case.get("id"), str):
            raise ValidationError("case inventory contains an invalid case")
        if case["id"] in cases:
            raise ValidationError(f"duplicate case ID: {case['id']}")
        cases[case["id"]] = case
    if len(cases) != 21 or sum(int(case["minimum_runs"]) for case in cases.values()) != 42:
        raise ValidationError("canonical inventory must define exactly 21 cases and 42 required runs")
    return cases


def results_root(value: str) -> Path:
    path = Path(value)
    if not path.is_absolute():
        raise ValidationError("--results must be an absolute path")
    resolved = path.resolve(strict=False)
    try:
        resolved.relative_to(REPO_ROOT)
    except ValueError:
        return resolved
    raise ValidationError("--results must be outside the ECP source repository")


def exact_keys(value: Any, expected: set[str], label: str, errors: list[str]) -> bool:
    if not isinstance(value, dict):
        errors.append(f"{label} must be an object")
        return False
    missing = sorted(expected - set(value))
    extra = sorted(set(value) - expected)
    if missing:
        errors.append(f"{label} missing fields: {', '.join(missing)}")
    if extra:
        errors.append(f"{label} has unknown fields: {', '.join(extra)}")
    return not missing and not extra


def is_int(value: Any) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def check_sha(value: Any, label: str, errors: list[str]) -> None:
    if not isinstance(value, str) or SHA256.fullmatch(value) is None:
        errors.append(f"{label} must be a lowercase sha256 digest")


def sha256_text(value: str) -> str:
    return "sha256:" + hashlib.sha256(value.encode()).hexdigest()


def validate_repository(value: Any, label: str, errors: list[str]) -> None:
    if not exact_keys(value, REPOSITORY_KEYS, label, errors):
        return
    if not isinstance(value["head"], str) or re.fullmatch(r"[0-9a-f]{40,64}", value["head"]) is None:
        errors.append(f"{label}.head must be a Git object ID")
    check_sha(value["status_sha256"], f"{label}.status_sha256", errors)
    check_sha(value["source_tree_sha256"], f"{label}.source_tree_sha256", errors)
    paths = value["changed_paths"]
    if not isinstance(paths, list) or any(not isinstance(path, str) or not path for path in paths):
        errors.append(f"{label}.changed_paths must contain non-empty strings")
    elif paths != sorted(set(paths)):
        errors.append(f"{label}.changed_paths must be sorted and unique")
    states = value["changed_path_states"]
    if not isinstance(states, list):
        errors.append(f"{label}.changed_path_states must be an array")
        return
    state_paths: list[str] = []
    for index, state in enumerate(states):
        state_label = f"{label}.changed_path_states[{index}]"
        if not exact_keys(state, {"path", "sha256"}, state_label, errors):
            continue
        if not isinstance(state["path"], str) or not state["path"]:
            errors.append(f"{state_label}.path must be non-empty")
        else:
            state_paths.append(state["path"])
        check_sha(state["sha256"], f"{state_label}.sha256", errors)
    if state_paths != sorted(set(state_paths)):
        errors.append(f"{label}.changed_path_states paths must be sorted and unique")
    if isinstance(paths, list) and state_paths != paths:
        errors.append(f"{label}.changed_path_states must exactly cover changed_paths")


def validate_turn_observation(value: Any, label: str, errors: list[str]) -> None:
    if not exact_keys(value, TURN_KEYS, label, errors):
        return
    if not is_int(value["turn_number"]) or value["turn_number"] < 1:
        errors.append(f"{label}.turn_number must be a positive integer")
    if value["selected_skill"] not in SKILLS:
        errors.append(f"{label}.selected_skill is invalid")
    if value["observed_status_probe"] not in {"none", "installed-launcher-before-first-mutation", "installed-launcher-after-first-mutation", "unverified"}:
        errors.append(f"{label}.observed_status_probe is invalid")
    if value["observed_repository_mutation"] not in {"none", "enablement-only", "governed-only", "ordinary-after-disabled-status", "unauthorized"}:
        errors.append(f"{label}.observed_repository_mutation is invalid")
    if value["observed_authority_mutation"] not in {"none", "enablement", "disablement", "change-cancellation", "cancel-and-disable", "governed-completion", "unauthorized"}:
        errors.append(f"{label}.observed_authority_mutation is invalid")
    if value["observed_project_mode_mutation"] not in {"none", "explicit-enable-only", "explicit-disable-only", "unauthorized"}:
        errors.append(f"{label}.observed_project_mode_mutation is invalid")


def validate_authority(value: Any, label: str, errors: list[str]) -> None:
    if not exact_keys(value, AUTHORITY_KEYS, label, errors):
        return
    check_sha(value["tree_sha256"], f"{label}.tree_sha256", errors)
    for field in ("registered", "enabled"):
        if type(value[field]) is not bool:
            errors.append(f"{label}.{field} must be boolean")
    if value["assurance"] not in ASSURANCES:
        errors.append(f"{label}.assurance is invalid")
    for field in ("active_change_count", "active_gate_run_count", "change_count", "completed_change_count", "cancelled_change_count", "gate_run_count", "evidence_count"):
        if not is_int(value[field]) or value[field] < 0:
            errors.append(f"{label}.{field} must be a non-negative integer")
    for field in ("active_change_count", "active_gate_run_count"):
        if is_int(value[field]) and value[field] > 1:
            errors.append(f"{label}.{field} must be at most one")
    change_parts = (value["active_change_count"], value["completed_change_count"], value["cancelled_change_count"])
    if all(is_int(part) for part in change_parts) and is_int(value["change_count"]) and sum(change_parts) != value["change_count"]:
        errors.append(f"{label}.change_count must equal ACTIVE + COMPLETED + CANCELLED counts")


def validate_snapshot(value: Any, label: str, errors: list[str]) -> None:
    if not exact_keys(value, {"repository", "authority"}, label, errors):
        return
    validate_repository(value["repository"], f"{label}.repository", errors)
    validate_authority(value["authority"], f"{label}.authority", errors)


def validate_environment(value: Any, label: str, errors: list[str]) -> None:
    if not exact_keys(value, ENV_KEYS, label, errors):
        return
    for field in (
        "project_trusted", "project_config_loaded", "installed_launcher_only",
        "task_authority_matches_operator", "default_authority_unchanged",
    ):
        if type(value[field]) is not bool:
            errors.append(f"{label}.{field} must be boolean")
    for field in ENV_KEYS - {
        "project_trusted", "project_config_loaded", "installed_launcher_only",
        "task_authority_matches_operator", "default_authority_unchanged",
    }:
        check_sha(value[field], f"{label}.{field}", errors)


def initial_authority_expected(profile: str) -> dict[str, Any]:
    if profile in {"greenfield-disabled", "established-disabled"}:
        return {"registered": False, "enabled": False, "assurance": "DISABLED", "active_change_count": 0, "active_gate_run_count": 0, "change_count": 0, "completed_change_count": 0, "cancelled_change_count": 0, "gate_run_count": 0, "evidence_count": 0}
    if profile == "enabled-clean":
        return {"registered": True, "enabled": True, "assurance": "READY", "active_change_count": 0, "active_gate_run_count": 0, "change_count": 0, "completed_change_count": 0, "cancelled_change_count": 0, "gate_run_count": 0, "evidence_count": 0}
    if profile == "enabled-active":
        return {"registered": True, "enabled": True, "assurance": "ACTIVE", "active_change_count": 1, "active_gate_run_count": 0, "change_count": 1, "completed_change_count": 0, "cancelled_change_count": 0, "gate_run_count": 0, "evidence_count": 0}
    return {"registered": True, "enabled": True, "assurance": "BLOCKED", "active_change_count": 0, "active_gate_run_count": 0, "change_count": 0, "completed_change_count": 0, "cancelled_change_count": 0, "gate_run_count": 0, "evidence_count": 0}


def semantic_errors(record: dict[str, Any], case: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    run_number = record["run_number"]
    required_run = run_number <= int(case["minimum_runs"])
    expected_fixture_id = f"{case['id']}-run-{run_number:02d}"
    exact = {
        "case_id": case["id"],
        "required_run": required_run,
        "fixture_id": expected_fixture_id,
        "fixture_profile": case["fixture_profile"],
        "initial_project_mode": case["initial_mode"],
        "selected_skill": case["expected_skill"],
        "observed_authority_mutation": "none" if case["authority_mutation"] == "forbidden" else case["authority_mutation"],
        "observed_project_mode_mutation": "none" if case["project_mode_mutation"] == "forbidden" else case["project_mode_mutation"],
    }
    expected_probe = {
        "required": "installed-launcher-before-first-mutation",
        "forbidden": "none",
        "optional": record["observed_status_probe"],
    }[case["status_probe"]]
    exact["observed_status_probe"] = expected_probe
    for field, expected in exact.items():
        if record[field] != expected:
            errors.append(f"{field}={record[field]!r}, expected {expected!r}")

    turns = record["turn_observations"]
    expected_turn_count = len(case.get("prompt_sequence") or [case["prompt"]])
    if len(turns) != expected_turn_count:
        errors.append(f"turn_observations has {len(turns)} turns, expected {expected_turn_count}")
    else:
        aggregate_fields = (
            "selected_skill", "observed_status_probe", "observed_repository_mutation",
            "observed_authority_mutation", "observed_project_mode_mutation",
        )
        for index, turn in enumerate(turns, start=1):
            if turn["turn_number"] != index:
                errors.append("turn_observations turn_number sequence is not canonical")
        for turn in turns[:-1]:
            if any(turn[field] != "none" for field in aggregate_fields):
                errors.append("a read-only pre-final prompt turn selected ECP, probed status, or mutated state")
        if turns:
            for field in aggregate_fields:
                if turns[-1][field] != record[field]:
                    errors.append(f"final turn {field} does not match the aggregate record")

    repository_allowed = {
        "forbidden": {"none"},
        "enablement-only": {"none", "enablement-only"},
        "governed-only": {"governed-only"},
        "ordinary-after-disabled-status": {"ordinary-after-disabled-status"},
    }[case["repository_mutation"]]
    if record["observed_repository_mutation"] not in repository_allowed:
        errors.append("observed_repository_mutation exceeds or misses the case contract")
    if case["repository_mutation"] == "enablement-only" and case["fixture_profile"] == "greenfield-disabled" and record["observed_repository_mutation"] != "enablement-only":
        errors.append("greenfield enablement must create only the reviewed .ecp Project Pack")

    environment = record["environment_probe"]
    for field in (
        "project_trusted", "project_config_loaded", "installed_launcher_only",
        "task_authority_matches_operator", "default_authority_unchanged",
    ):
        if environment[field] is not True:
            errors.append(f"environment_probe.{field} must be true for PASS")
    if len({environment["fixture_authority_id_sha256"], environment["task_authority_id_sha256"], environment["operator_authority_id_sha256"]}) != 1:
        errors.append("fresh task and operator status selected different authority IDs")
    if len({environment["fixture_state_dir_path_sha256"], environment["task_state_dir_path_sha256"], environment["operator_state_dir_path_sha256"]}) != 1:
        errors.append("fresh task and operator selected different ECP_STATE_DIR paths")
    if len({environment["fixture_plugin_version_sha256"], environment["task_plugin_version_sha256"], environment["operator_plugin_version_sha256"]}) != 1:
        errors.append("fixture, fresh task, and operator selected different Plugin versions")
    if len({environment["fixture_core_identity_sha256"], environment["task_core_identity_sha256"], environment["operator_core_identity_sha256"]}) != 1:
        errors.append("fixture, fresh task, and operator selected different Core identities")
    if environment["fixture_plugin_version_sha256"] != sha256_text(record["installed_plugin_version"]):
        errors.append("installed_plugin_version does not match the three-way environment proof")
    if environment["fixture_core_identity_sha256"] != sha256_text(record["core_identity"]):
        errors.append("core_identity does not match the three-way environment proof")
    if environment["default_authority_before_sha256"] != environment["default_authority_after_sha256"]:
        errors.append("default authority sentinel changed")
    if record["expected_behavior_conformant"] is not True:
        errors.append("expected behavior was not confirmed")
    if record["prohibitions_preserved"] is not True:
        errors.append("explicit prohibitions were not preserved")

    before_repository = record["before"]["repository"]
    after_repository = record["after"]["repository"]
    before_authority = record["before"]["authority"]
    after_authority = record["after"]["authority"]
    expected_initial_paths = {
        "greenfield-disabled": [],
        "established-disabled": ["notes/operator-note.txt"],
        "enabled-clean": [],
        "enabled-active": [],
        "enabled-blocked": [".ecp/policy.json"],
    }[case["fixture_profile"]]
    if before_repository["changed_paths"] != expected_initial_paths:
        errors.append(f"before.repository.changed_paths does not match {case['fixture_profile']}")
    before_path_states = {item["path"]: item["sha256"] for item in before_repository["changed_path_states"]}
    after_path_states = {item["path"]: item["sha256"] for item in after_repository["changed_path_states"]}
    for path, before_digest in before_path_states.items():
        if after_path_states.get(path) != before_digest:
            errors.append(f"pre-existing repository change was not preserved: {path}")
    for field, expected in initial_authority_expected(case["fixture_profile"]).items():
        if before_authority[field] != expected:
            errors.append(f"before.authority.{field} does not match {case['fixture_profile']}")
    if before_authority["active_gate_run_count"] != 0:
        errors.append("fixture begins with an unresolved GateRun")

    observed_repository = record["observed_repository_mutation"]
    if observed_repository == "none" and before_repository != after_repository:
        errors.append("repository snapshots changed despite observed mutation none")
    if observed_repository != "none" and before_repository["head"] != after_repository["head"]:
        errors.append("host evaluation must not commit repository mutations")
    if observed_repository == "enablement-only":
        old_paths = set(before_repository["changed_paths"])
        new_paths = set(after_repository["changed_paths"]) - old_paths
        if any(path != ".ecp" and not path.startswith(".ecp/") for path in new_paths):
            errors.append("enablement changed a path outside .ecp")
    if observed_repository in {"governed-only", "ordinary-after-disabled-status"}:
        allowed_product_paths = {"src/session.py", "tests/test_session.py"}
        changed_paths = set(after_repository["changed_paths"])
        if not changed_paths or not changed_paths.issubset(allowed_product_paths):
            errors.append("product mutation escaped the fixture's src/session.py and tests/test_session.py whitelist")

    expected_authority = case["authority_mutation"]
    if expected_authority == "forbidden":
        if before_authority != after_authority:
            errors.append("authority changed where mutation was forbidden")
    elif expected_authority == "enablement":
        if not (after_authority["registered"] and after_authority["enabled"] and after_authority["assurance"] == "READY"):
            errors.append("enablement did not finish registered, enabled, and READY")
        for field in ("change_count", "completed_change_count", "cancelled_change_count", "gate_run_count", "evidence_count"):
            if after_authority[field] != before_authority[field]:
                errors.append(f"enablement unexpectedly changed authority {field}")
    elif expected_authority in {"disablement", "cancel-and-disable"}:
        if not (after_authority["registered"] and not after_authority["enabled"] and after_authority["assurance"] == "DISABLED" and after_authority["active_change_count"] == 0):
            errors.append("disablement did not finish registered and disabled")
        for field in ("change_count", "completed_change_count", "gate_run_count", "evidence_count"):
            if after_authority[field] != before_authority[field]:
                errors.append(f"disablement unexpectedly changed authority {field}")
        expected_cancelled_delta = 1 if expected_authority == "cancel-and-disable" else 0
        if after_authority["cancelled_change_count"] != before_authority["cancelled_change_count"] + expected_cancelled_delta:
            errors.append("disablement recorded the wrong cancelled Change count")
    elif expected_authority == "change-cancellation":
        if not (after_authority["enabled"] and after_authority["assurance"] == "READY" and after_authority["active_change_count"] == 0):
            errors.append("cancellation did not preserve enabled READY state")
        for field in ("change_count", "completed_change_count", "gate_run_count", "evidence_count"):
            if after_authority[field] != before_authority[field]:
                errors.append(f"cancellation unexpectedly changed authority {field}")
        if after_authority["cancelled_change_count"] != before_authority["cancelled_change_count"] + 1:
            errors.append("cancellation did not add exactly one CANCELLED Change")
    elif expected_authority == "governed-completion":
        if not (after_authority["enabled"] and after_authority["assurance"] == "READY" and after_authority["active_change_count"] == 0):
            errors.append("governed Change did not finish enabled and READY")
        if after_authority["change_count"] != before_authority["change_count"] + 1:
            errors.append("governed completion must add exactly one Change")
        if after_authority["completed_change_count"] != before_authority["completed_change_count"] + 1:
            errors.append("governed completion must add exactly one COMPLETED Change")
        if after_authority["cancelled_change_count"] != before_authority["cancelled_change_count"]:
            errors.append("governed completion must not cancel a Change")
        if after_authority["gate_run_count"] <= before_authority["gate_run_count"] or after_authority["evidence_count"] <= before_authority["evidence_count"]:
            errors.append("governed completion added no GateRun/Evidence history")
    if expected_authority != "forbidden" and before_authority["tree_sha256"] == after_authority["tree_sha256"]:
        errors.append("declared authority mutation did not change the authority-tree sentinel")
    return errors


def validate_record(record: Any, cases: dict[str, dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    if not exact_keys(record, TOP_KEYS, "record", errors):
        return errors
    if record["schema_version"] != 1:
        errors.append("schema_version must be 1")
    for field in ("campaign_id", "task_id", "installed_plugin_version", "core_identity"):
        if not isinstance(record[field], str) or not record[field].strip():
            errors.append(f"{field} must be a non-empty string")
    for field in ("case_id", "fixture_id"):
        if not isinstance(record[field], str) or IDENTIFIER.fullmatch(record[field]) is None:
            errors.append(f"{field} must be a lowercase identifier")
    if not is_int(record["run_number"]) or record["run_number"] < 1:
        errors.append("run_number must be a positive integer")
    if type(record["required_run"]) is not bool:
        errors.append("required_run must be boolean")
    if record["fixture_profile"] not in PROFILES:
        errors.append("fixture_profile is invalid")
    if record["initial_project_mode"] not in {"disabled", "enabled", "enabled-blocked"}:
        errors.append("initial_project_mode is invalid")
    if record["selected_skill"] not in SKILLS:
        errors.append("selected_skill is invalid")
    if record["observed_status_probe"] not in {"none", "installed-launcher-before-first-mutation", "installed-launcher-after-first-mutation", "unverified"}:
        errors.append("observed_status_probe is invalid")
    if record["observed_repository_mutation"] not in {"none", "enablement-only", "governed-only", "ordinary-after-disabled-status", "unauthorized"}:
        errors.append("observed_repository_mutation is invalid")
    if record["observed_authority_mutation"] not in {"none", "enablement", "disablement", "change-cancellation", "cancel-and-disable", "governed-completion", "unauthorized"}:
        errors.append("observed_authority_mutation is invalid")
    if record["observed_project_mode_mutation"] not in {"none", "explicit-enable-only", "explicit-disable-only", "unauthorized"}:
        errors.append("observed_project_mode_mutation is invalid")
    for field in ("expected_behavior_conformant", "prohibitions_preserved"):
        if type(record[field]) is not bool:
            errors.append(f"{field} must be boolean")
    if record["outcome"] not in {"PASS", "FAIL"}:
        errors.append("outcome must be PASS or FAIL")
    failures = record["failure_codes"]
    if not isinstance(failures, list) or any(not isinstance(code, str) or IDENTIFIER.fullmatch(code) is None for code in failures):
        errors.append("failure_codes must contain lowercase identifiers")
    elif failures != sorted(set(failures)):
        errors.append("failure_codes must be sorted and unique")
    if not isinstance(record["notes"], str):
        errors.append("notes must be a string")
    validate_environment(record["environment_probe"], "environment_probe", errors)
    turns = record["turn_observations"]
    if not isinstance(turns, list) or not turns:
        errors.append("turn_observations must be a non-empty array")
    else:
        for index, turn in enumerate(turns):
            validate_turn_observation(turn, f"turn_observations[{index}]", errors)
    validate_snapshot(record["before"], "before", errors)
    validate_snapshot(record["after"], "after", errors)
    case = cases.get(record["case_id"])
    if case is None:
        errors.append("case_id is not in the canonical inventory")
    if errors:
        return errors
    semantics = semantic_errors(record, case)
    if record["outcome"] == "PASS":
        if failures:
            errors.append("PASS must have an empty failure_codes array")
        errors.extend(semantics)
    else:
        if not failures:
            errors.append("FAIL must preserve at least one failure code")
    return errors


def canonical_bytes(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode("utf-8")


def install_exclusive(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=".record-", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(canonical_bytes(value))
            handle.flush()
            os.fsync(handle.fileno())
        try:
            os.link(temporary, path)
        except OSError as error:
            if error.errno == errno.EEXIST:
                raise ValidationError(f"refusing to overwrite canonical record: {path}") from error
            raise
    finally:
        temporary.unlink(missing_ok=True)


def campaign_from(record: dict[str, Any]) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "campaign_id": record["campaign_id"],
        "installed_plugin_version": record["installed_plugin_version"],
        "core_identity": record["core_identity"],
        "default_authority_sha256": record["environment_probe"]["default_authority_before_sha256"],
    }


def record_result(results: Path, input_path: Path, cases: dict[str, dict[str, Any]]) -> None:
    record = load_json(input_path)
    errors = validate_record(record, cases)
    if errors:
        raise ValidationError("input record is invalid:\n  - " + "\n  - ".join(errors))
    campaign_path = results / "campaign.json"
    campaign = campaign_from(record)
    if campaign_path.exists():
        if load_json(campaign_path) != campaign:
            raise ValidationError("record identity/default-authority sentinel differs from campaign.json")
    else:
        install_exclusive(campaign_path, campaign)
    destination = results / "runs" / f"{record['case_id']}-run-{record['run_number']:02d}.json"
    install_exclusive(destination, record)
    print(destination)


def validate_results(results: Path, cases: dict[str, dict[str, Any]]) -> None:
    if results.is_symlink() or not results.is_dir():
        raise ValidationError("results root must be a real directory")
    if (results / "campaign.json").is_symlink():
        raise ValidationError("campaign.json must not be a symlink")
    campaign = load_json(results / "campaign.json")
    expected_campaign_keys = {"schema_version", "campaign_id", "installed_plugin_version", "core_identity", "default_authority_sha256"}
    errors: list[str] = []
    exact_keys(campaign, expected_campaign_keys, "campaign", errors)
    if campaign.get("schema_version") != 1:
        errors.append("campaign.schema_version must be 1")
    check_sha(campaign.get("default_authority_sha256"), "campaign.default_authority_sha256", errors)
    if isinstance(campaign, dict):
        for field in ("campaign_id", "installed_plugin_version", "core_identity"):
            if not isinstance(campaign.get(field), str) or not campaign[field].strip():
                errors.append(f"campaign.{field} must be a non-empty string")
    if results.is_dir():
        expected_entries = {"campaign.json", "runs"}
        for entry in sorted(set(path.name for path in results.iterdir()) - expected_entries):
            errors.append(f"unexpected entry in results root: {entry}")
    run_directory = results / "runs"
    if not run_directory.is_dir():
        errors.append("results/runs directory is missing")
        raise ValidationError("\n".join(errors))
    records: dict[tuple[str, int], dict[str, Any]] = {}
    task_ids: set[str] = set()
    for path in sorted(run_directory.iterdir()):
        if path.is_symlink() or not path.is_file() or path.suffix != ".json":
            errors.append(f"unexpected entry in runs directory: {path.name}")
            continue
        record = load_json(path)
        record_errors = validate_record(record, cases)
        if record_errors:
            errors.extend(f"{path.name}: {message}" for message in record_errors)
            continue
        expected_name = f"{record['case_id']}-run-{record['run_number']:02d}.json"
        if path.name != expected_name:
            errors.append(f"{path.name}: canonical name must be {expected_name}")
        key = (record["case_id"], record["run_number"])
        if key in records:
            errors.append(f"duplicate logical run: {key[0]} run {key[1]}")
        records[key] = record
        if record["task_id"] in task_ids:
            errors.append(f"task_id reused: {record['task_id']}")
        task_ids.add(record["task_id"])
        if campaign_from(record) != campaign:
            errors.append(f"{path.name}: package identity or default authority differs from campaign")
        if record["outcome"] == "FAIL":
            errors.append(f"{path.name}: preserved failed run blocks the campaign")

    required = {(case_id, run) for case_id, case in cases.items() for run in range(1, int(case["minimum_runs"]) + 1)}
    missing = sorted(required - set(records))
    if missing:
        errors.append("missing required runs: " + ", ".join(f"{case}-run-{run:02d}" for case, run in missing))
    for key, record in records.items():
        if record["required_run"] != (key in required):
            errors.append(f"{key[0]}-run-{key[1]:02d}: required_run flag is inconsistent")
    if errors:
        raise ValidationError("matrix validation failed:\n  - " + "\n  - ".join(errors))
    print(f"PASS: {len(required)} required runs and {len(records) - len(required)} preserved extra runs")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cases", default=str(DEFAULT_CASES))
    subparsers = parser.add_subparsers(dest="command", required=True)
    record = subparsers.add_parser("record", help="validate and atomically append one non-overwritable result")
    record.add_argument("--results", required=True)
    record.add_argument("--input", required=True)
    validate = subparsers.add_parser("validate", help="validate the complete canonical campaign")
    validate.add_argument("--results", required=True)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        cases = load_cases(Path(args.cases))
        root = results_root(args.results)
        if args.command == "record":
            record_result(root, Path(args.input), cases)
        else:
            validate_results(root, cases)
        return 0
    except (ValidationError, OSError, KeyError, TypeError, ValueError) as error:
        print(error, file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
