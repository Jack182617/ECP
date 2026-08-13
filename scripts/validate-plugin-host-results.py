#!/usr/bin/env python3
"""Record and validate the append-only fail-fast Plugin host qualification."""

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
    "fixture_state_dir_path_sha256", "operator_state_dir_path_sha256",
    "fixture_authority_id_sha256", "operator_authority_id_sha256",
    "fixture_plugin_version_sha256", "operator_plugin_version_sha256",
    "fixture_core_identity_sha256", "operator_core_identity_sha256",
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
MAX_QUALIFICATION_ATTEMPTS = 2


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
    if not isinstance(inventory, dict) or inventory.get("schema_version") != 3:
        raise ValidationError("case inventory must use schema_version 3")
    cases: dict[str, dict[str, Any]] = {}
    for case in inventory.get("cases", []):
        if not isinstance(case, dict) or not isinstance(case.get("id"), str):
            raise ValidationError("case inventory contains an invalid case")
        if case["id"] in cases:
            raise ValidationError(f"duplicate case ID: {case['id']}")
        cases[case["id"]] = case
    if len(cases) != 21:
        raise ValidationError("canonical inventory must define exactly 21 cases")
    qualification = [case for case in cases.values() if case.get("suite") == "qualification"]
    extended = [case for case in cases.values() if case.get("suite") == "extended"]
    if len(qualification) != 12 or len(extended) != 9:
        raise ValidationError("canonical inventory must define 12 qualification and 9 extended cases")
    orders = sorted(case.get("qualification_order") for case in qualification)
    if orders != list(range(1, 13)):
        raise ValidationError("qualification_order must contain each integer from 1 through 12 exactly once")
    if any("qualification_order" in case for case in extended):
        raise ValidationError("extended cases must not define qualification_order")
    return cases


def qualification_cases(cases: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    return sorted(
        (case for case in cases.values() if case["suite"] == "qualification"),
        key=lambda case: case["qualification_order"],
    )


def is_within(path: Path, root: Path) -> bool:
    try:
        path.relative_to(root)
        return True
    except ValueError:
        return False


def results_root(value: str) -> Path:
    path = Path(value)
    if not path.is_absolute():
        raise ValidationError("--results must be an absolute path")
    resolved = path.resolve(strict=False)
    forbidden_roots = {
        REPO_ROOT.resolve(),
        Path(tempfile.gettempdir()).resolve(),
        Path("/tmp").resolve(),
        Path("/private/tmp").resolve(),
        (Path.home() / ".ecp").resolve(),
        (Path.home() / ".codex" / "plugins" / "cache").resolve(),
    }
    if any(is_within(resolved, root) for root in forbidden_roots):
        raise ValidationError("--results must be a durable root outside source, temporary, authority, and cache trees")
    if any(part.lower() in {"authority", "cache"} for part in resolved.parts):
        raise ValidationError("--results must not be inside an authority or cache directory")
    return resolved


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
        "project_trusted", "project_config_loaded", "installed_launcher_only", "default_authority_unchanged",
    ):
        if type(value[field]) is not bool:
            errors.append(f"{label}.{field} must be boolean")
    for field in ENV_KEYS - {
        "project_trusted", "project_config_loaded", "installed_launcher_only", "default_authority_unchanged",
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
    required_run = case["suite"] == "qualification" and run_number == 1
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
        "project_trusted", "project_config_loaded", "installed_launcher_only", "default_authority_unchanged",
    ):
        if environment[field] is not True:
            errors.append(f"environment_probe.{field} must be true for PASS")
    if environment["fixture_authority_id_sha256"] != environment["operator_authority_id_sha256"]:
        errors.append("fixture and operator status selected different authority IDs")
    if environment["fixture_state_dir_path_sha256"] != environment["operator_state_dir_path_sha256"]:
        errors.append("fixture and operator selected different ECP_STATE_DIR paths")
    if environment["fixture_plugin_version_sha256"] != environment["operator_plugin_version_sha256"]:
        errors.append("fixture and operator selected different Plugin versions")
    if environment["fixture_core_identity_sha256"] != environment["operator_core_identity_sha256"]:
        errors.append("fixture and operator selected different Core identities")
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
    if record["schema_version"] != 2:
        errors.append("schema_version must be 2")
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
    if record["outcome"] not in {"PASS", "FAIL", "INVALID"}:
        errors.append("outcome must be PASS, FAIL, or INVALID")
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
    elif record["outcome"] == "FAIL":
        if not failures:
            errors.append("FAIL must preserve at least one failure code")
    else:
        if not failures:
            errors.append("INVALID must preserve at least one failure code")
        elif not any(code.startswith("infra-") for code in failures):
            errors.append("INVALID must include an infra- failure code")
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
        "schema_version": 2,
        "campaign_id": record["campaign_id"],
        "installed_plugin_version": record["installed_plugin_version"],
        "core_identity": record["core_identity"],
        "default_authority_sha256": record["environment_probe"]["default_authority_before_sha256"],
        "protocol": "serial-fail-fast-qualification-v1",
        "qualification_case_count": 12,
    }


def scan_records(
    results: Path, cases: dict[str, dict[str, Any]], require_directory: bool = False
) -> tuple[dict[tuple[str, int], dict[str, Any]], list[str]]:
    records: dict[tuple[str, int], dict[str, Any]] = {}
    errors: list[str] = []
    task_ids: set[str] = set()
    run_directory = results / "runs"
    if not run_directory.exists() and not require_directory:
        return records, errors
    if run_directory.is_symlink() or not run_directory.is_dir():
        return records, ["results/runs directory must be a real directory"]
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
    return records, errors


def qualification_state(
    records: dict[tuple[str, int], dict[str, Any]], cases: dict[str, dict[str, Any]]
) -> tuple[str, str | None, list[str]]:
    errors: list[str] = []
    blocker: str | None = None
    failed_case: str | None = None
    invalid_case: str | None = None
    completed = 0
    stopped = False
    for case in qualification_cases(cases):
        case_id = case["id"]
        attempts = sorted(
            (record for (record_case, _), record in records.items() if record_case == case_id),
            key=lambda record: record["run_number"],
        )
        numbers = [record["run_number"] for record in attempts]
        if numbers and numbers != list(range(1, len(numbers) + 1)):
            errors.append(f"{case_id}: attempts must use contiguous run numbers starting at 1")
        if len(attempts) > MAX_QUALIFICATION_ATTEMPTS:
            errors.append(f"{case_id}: qualification allows at most one fresh-fixture retry after INVALID")
        if attempts:
            for index, attempt in enumerate(attempts):
                expected_required = index == 0
                if attempt["required_run"] is not expected_required:
                    errors.append(f"{case_id}-run-{attempt['run_number']:02d}: required_run flag is inconsistent")
            terminal_indexes = [
                index for index, attempt in enumerate(attempts)
                if attempt["outcome"] in {"PASS", "FAIL"}
            ]
            if len(terminal_indexes) > 1 or (terminal_indexes and terminal_indexes[0] != len(attempts) - 1):
                errors.append(f"{case_id}: PASS or FAIL must be the final attempt; only INVALID may precede it")
            if any(attempt["outcome"] != "INVALID" for attempt in attempts[:-1]):
                errors.append(f"{case_id}: only INVALID attempts may precede the final attempt")
        if stopped and attempts:
            errors.append(f"{case_id}: qualification is serial and cannot start after an earlier incomplete or failed case")
            continue
        if not attempts:
            stopped = True
            if blocker is None:
                blocker = case_id
            continue
        final = attempts[-1]["outcome"]
        if final == "PASS":
            completed += 1
            continue
        stopped = True
        blocker = case_id
        if final == "FAIL":
            failed_case = case_id
        elif len(attempts) == MAX_QUALIFICATION_ATTEMPTS:
            invalid_case = case_id
    if failed_case is not None:
        return "FAILED", failed_case, errors
    if invalid_case is not None:
        return "INVALID", invalid_case, errors
    if completed == len(qualification_cases(cases)):
        return "QUALIFIED", None, errors
    return "INCOMPLETE", blocker, errors


def enforce_next_record(
    record: dict[str, Any], records: dict[tuple[str, int], dict[str, Any]], cases: dict[str, dict[str, Any]]
) -> None:
    state, blocker, errors = qualification_state(records, cases)
    if errors:
        raise ValidationError("existing qualification history is invalid:\n  - " + "\n  - ".join(errors))
    if state == "FAILED":
        raise ValidationError("qualification is already FAILED; product FAIL is terminal")
    if state == "INVALID":
        raise ValidationError(
            "qualification is already INVALID; the one fresh-fixture retry also failed infrastructure"
        )
    case = cases[record["case_id"]]
    existing_numbers = sorted(run for case_id, run in records if case_id == record["case_id"])
    expected_number = (existing_numbers[-1] + 1) if existing_numbers else 1
    if record["run_number"] != expected_number:
        raise ValidationError(
            f"{record['case_id']}: next append must use run {expected_number:02d}, got {record['run_number']:02d}"
        )
    expected_required = case["suite"] == "qualification" and record["run_number"] == 1
    if record["required_run"] is not expected_required:
        raise ValidationError("required_run must be true only for a qualification case's first attempt")
    if case["suite"] == "qualification":
        if state == "QUALIFIED":
            raise ValidationError("qualification is already QUALIFIED; no further qualification attempt is allowed")
        if record["case_id"] != blocker:
            raise ValidationError(f"qualification is serial; next case must be {blocker}")
        prior = [records[(record["case_id"], run)] for run in existing_numbers]
        if prior and prior[-1]["outcome"] != "INVALID":
            raise ValidationError("a fresh retry is allowed only after INVALID")
    elif state != "QUALIFIED":
        raise ValidationError("extended diagnostics may run only after qualification is QUALIFIED")


def record_result(results: Path, input_path: Path, cases: dict[str, dict[str, Any]]) -> None:
    record = load_json(input_path)
    errors = validate_record(record, cases)
    if errors:
        raise ValidationError("input record is invalid:\n  - " + "\n  - ".join(errors))
    records, existing_errors = scan_records(results, cases)
    if existing_errors:
        raise ValidationError("existing result records are invalid:\n  - " + "\n  - ".join(existing_errors))
    enforce_next_record(record, records, cases)
    campaign_path = results / "campaign.json"
    campaign = campaign_from(record)
    if campaign_path.exists():
        if campaign_path.is_symlink() or load_json(campaign_path) != campaign:
            raise ValidationError("record identity/default-authority sentinel differs from campaign.json")
    else:
        install_exclusive(campaign_path, campaign)
    destination = results / "runs" / f"{record['case_id']}-run-{record['run_number']:02d}.json"
    install_exclusive(destination, record)
    records[(record["case_id"], record["run_number"])] = record
    state, blocker, _ = qualification_state(records, cases)
    suffix = f"; next={blocker}" if blocker else ""
    print(f"{destination}\n{state}{suffix}")


def validate_results(results: Path, cases: dict[str, dict[str, Any]]) -> None:
    if results.is_symlink() or not results.is_dir():
        raise ValidationError("results root must be a real directory")
    if (results / "campaign.json").is_symlink():
        raise ValidationError("campaign.json must not be a symlink")
    campaign = load_json(results / "campaign.json")
    expected_campaign_keys = {
        "schema_version", "campaign_id", "installed_plugin_version", "core_identity",
        "default_authority_sha256", "protocol", "qualification_case_count",
    }
    errors: list[str] = []
    exact_keys(campaign, expected_campaign_keys, "campaign", errors)
    if campaign.get("schema_version") != 2:
        errors.append("campaign.schema_version must be 2")
    if campaign.get("protocol") != "serial-fail-fast-qualification-v1":
        errors.append("campaign.protocol is invalid")
    if campaign.get("qualification_case_count") != 12:
        errors.append("campaign.qualification_case_count must be 12")
    check_sha(campaign.get("default_authority_sha256"), "campaign.default_authority_sha256", errors)
    if isinstance(campaign, dict):
        for field in ("campaign_id", "installed_plugin_version", "core_identity"):
            if not isinstance(campaign.get(field), str) or not campaign[field].strip():
                errors.append(f"campaign.{field} must be a non-empty string")
    if results.is_dir():
        expected_entries = {"campaign.json", "runs"}
        for entry in sorted(set(path.name for path in results.iterdir()) - expected_entries):
            errors.append(f"unexpected entry in results root: {entry}")
    records, record_errors = scan_records(results, cases, require_directory=True)
    errors.extend(record_errors)
    for key, record in records.items():
        if campaign_from(record) != campaign:
            errors.append(f"{key[0]}-run-{key[1]:02d}: package identity or default authority differs from campaign")
        case = cases[record["case_id"]]
        expected_required = case["suite"] == "qualification" and record["run_number"] == 1
        if record["required_run"] is not expected_required:
            errors.append(f"{key[0]}-run-{key[1]:02d}: required_run flag is inconsistent")
    state, blocker, sequence_errors = qualification_state(records, cases)
    errors.extend(sequence_errors)
    if state != "QUALIFIED" and any(cases[case_id]["suite"] == "extended" for case_id, _ in records):
        errors.append("extended diagnostics were recorded before qualification reached QUALIFIED")
    if errors:
        raise ValidationError("qualification validation failed:\n  - " + "\n  - ".join(errors))
    invalid_count = sum(record["outcome"] == "INVALID" for record in records.values())
    extended_count = sum(cases[case_id]["suite"] == "extended" for case_id, _ in records)
    if state == "FAILED":
        raise ValidationError(f"FAILED: product behavior failed at {blocker}; no later qualification case may run")
    if state == "INVALID":
        raise ValidationError(
            f"INVALID: infrastructure failed twice at {blocker}; freeze this candidate campaign"
        )
    if state == "INCOMPLETE":
        raise ValidationError(f"INCOMPLETE: next qualification case is {blocker}")
    print(
        f"QUALIFIED: 12 qualification cases passed; "
        f"{invalid_count} INVALID attempts and {extended_count} extended diagnostics preserved"
    )


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
