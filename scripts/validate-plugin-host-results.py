#!/usr/bin/env python3
"""Record and validate the append-only fail-fast Plugin host qualification."""

from __future__ import annotations

import argparse
import errno
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import stat
import subprocess
import sys
import tempfile
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CASES = REPO_ROOT / "docs" / "plugin-host-evaluation-cases.json"
RESULT_SCHEMA = REPO_ROOT / "docs" / "plugin-host-evaluation-result.schema.json"
FIXTURE_BUILDER = REPO_ROOT / "scripts" / "plugin-host-fixture.py"
DEFAULT_AUTHORITY = Path.home() / ".ecp" / "state-v1"
SHA256 = re.compile(r"^sha256:[0-9a-f]{64}$")
IDENTIFIER = re.compile(r"^[a-z0-9][a-z0-9._-]*$")
CACHE_COMPONENT = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._+-]*$")
TOP_KEYS = {
    "schema_version", "campaign_id", "case_id", "run_number", "required_run",
    "task_id", "fixture_id", "fixture_profile", "initial_project_mode",
    "environment_probe", "desktop_build", "installed_plugin_version", "core_identity",
    "installed_plugin_tree_sha256", "installed_launcher_locator_sha256",
    "resolved_skill_locator_sha256", "prompt_sha256", "dispatch_intent_sha256",
    "fixture_metadata_sha256", "workspace_path_sha256", "task_ledger_sha256",
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
QUALIFICATION_CASE_COUNT = 16
EXTENDED_CASE_COUNT = 5
INFRA_FAILURE_CODES = {
    "infra-desktop-unavailable",
    "infra-dispatch-failed",
    "infra-dispatch-timeout",
    "infra-fixture-invalid",
    "infra-host-crash",
    "infra-observation-incomplete",
    "infra-project-config-unavailable",
    "infra-project-trust-unavailable",
    "infra-task-identity-ambiguous",
    "infra-task-ledger-unavailable",
}
PRODUCT_FAILURE_CODES = {
    "behavior-nonconformant",
    "candidate-identity-mismatch",
    "direct-authority-access",
    "forbidden-status-probe",
    "missing-required-status-probe",
    "oracle-used",
    "prohibition-breached",
    "unauthorized-authority-mutation",
    "unauthorized-project-mode-mutation",
    "unauthorized-repository-mutation",
    "workspace-mismatch",
    "wrong-launcher",
    "wrong-route",
}
CAMPAIGN_KEYS = {
    "schema_version", "campaign_id", "protocol", "desktop_build",
    "desktop_inventory_sha256", "installed_plugin_version", "core_identity",
    "installed_plugin_root_sha256", "installed_launcher_locator_sha256",
    "installed_plugin_tree_sha256", "runtime_manifest_sha256", "skill_locator_sha256",
    "case_inventory_sha256", "result_schema_sha256", "validator_sha256",
    "fixture_builder_sha256", "default_authority_sha256",
    "qualification_case_count",
}
LEDGER_KEYS = {
    "schema_version", "campaign_id", "case_id", "run_number", "task_id",
    "task_created", "fixture_id", "desktop_build", "workspace_path_sha256",
    "task_cwd_sha256", "prompt_sha256", "dispatch_intent_sha256",
    "fixture_metadata_sha256", "selected_skill", "resolved_skill_locator_sha256",
    "installed_launcher_locator_sha256", "installed_plugin_tree_sha256",
    "runtime_manifest_sha256",
    "rollout_log_sha256", "readback_source",
}
FIXTURE_METADATA_REQUIRED = {
    "schema_version", "campaign_id", "fixture_id", "case_id", "run_number",
    "required_run", "case_suite", "qualification_order", "fixture_profile",
    "workspace", "workspace_path_sha256", "dedicated_state_dir",
    "expected_state_dir_path_sha256", "project_config",
    "expected_authority_id_sha256", "installed_plugin_version",
    "desktop_build", "installed_plugin_root_sha256",
    "installed_launcher_locator_sha256", "installed_plugin_tree_sha256",
    "runtime_manifest_sha256",
    "skill_locator_sha256", "case_inventory_sha256", "fixture_builder_sha256",
    "core_version", "core_identity", "default_authority_before_sha256",
    "preparation", "prep_state", "prompt_sequence", "prompt_sha256",
    "prepared_authority_tree_sha256", "prepared_status",
}


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
    if not isinstance(inventory, dict) or inventory.get("schema_version") != 4:
        raise ValidationError("case inventory must use schema_version 4")
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
    if len(qualification) != QUALIFICATION_CASE_COUNT or len(extended) != EXTENDED_CASE_COUNT:
        raise ValidationError("canonical inventory must define 16 qualification and 5 extended cases")
    orders = sorted(case.get("qualification_order") for case in qualification)
    if orders != list(range(1, QUALIFICATION_CASE_COUNT + 1)):
        raise ValidationError("qualification_order must contain each integer from 1 through 16 exactly once")
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


def sha256_bytes(value: bytes) -> str:
    return "sha256:" + hashlib.sha256(value).hexdigest()


def sha256_file(path: Path) -> str:
    try:
        return sha256_bytes(path.read_bytes())
    except OSError as error:
        raise ValidationError(f"could not hash canonical file {path}: {error}") from error


def path_digest(path: Path) -> str:
    return sha256_text(str(path.resolve(strict=False)))


def tree_digest(root: Path) -> str:
    raw_root = root
    if raw_root.is_symlink():
        raise ValidationError(f"installed Plugin root is not a real directory: {raw_root}")
    root = raw_root.resolve(strict=True)
    if not root.is_dir():
        raise ValidationError(f"installed Plugin root is not a real directory: {root}")
    digest = hashlib.sha256()
    digest.update(b"directory\0")
    for path in sorted(root.rglob("*"), key=lambda item: item.relative_to(root).as_posix()):
        relative = path.relative_to(root).as_posix().encode()
        info = path.lstat()
        digest.update(relative + b"\0" + oct(stat.S_IMODE(info.st_mode)).encode() + b"\0")
        if stat.S_ISREG(info.st_mode):
            digest.update(b"file\0")
            with path.open("rb") as handle:
                while chunk := handle.read(1024 * 1024):
                    digest.update(chunk)
        elif stat.S_ISDIR(info.st_mode):
            digest.update(b"directory\0")
        else:
            raise ValidationError(f"installed Plugin tree contains a symlink or special entry: {path}")
    return "sha256:" + digest.hexdigest()


def sentinel_tree_digest(root: Path) -> str:
    """Hash authority state with the fixture builder's sentinel contract."""
    digest = hashlib.sha256()
    if not root.exists() and not root.is_symlink():
        digest.update(b"absent\0")
        return "sha256:" + digest.hexdigest()
    if root.is_symlink() or not root.is_dir():
        raise ValidationError(f"authority sentinel is not a real directory: {root}")
    digest.update(b"directory\0")
    for path in sorted(root.rglob("*"), key=lambda item: item.relative_to(root).as_posix()):
        relative = path.relative_to(root).as_posix().encode("utf-8")
        info = path.lstat()
        digest.update(relative + b"\0" + oct(stat.S_IMODE(info.st_mode)).encode() + b"\0")
        if stat.S_ISREG(info.st_mode):
            digest.update(b"file\0" + str(info.st_size).encode() + b"\0")
            with path.open("rb") as handle:
                while chunk := handle.read(1024 * 1024):
                    digest.update(chunk)
        elif stat.S_ISDIR(info.st_mode):
            digest.update(b"directory\0")
        elif stat.S_ISLNK(info.st_mode):
            digest.update(b"symlink\0" + os.readlink(path).encode("utf-8") + b"\0")
        else:
            digest.update(b"special\0")
    return "sha256:" + digest.hexdigest()


def case_prompt_digest(case: dict[str, Any]) -> str:
    prompts = case.get("prompt_sequence") or [case["prompt"]]
    return sha256_bytes(canonical_bytes(prompts))


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


def dispatch_intent_digest(record: dict[str, Any]) -> str:
    intent = {
        "campaign_id": record["campaign_id"],
        "case_id": record["case_id"],
        "fixture_id": record["fixture_id"],
        "prompt_sha256": record["prompt_sha256"],
        "run_number": record["run_number"],
        "workspace_path_sha256": record["workspace_path_sha256"],
    }
    return sha256_bytes(canonical_bytes(intent))


def canonical_contract_digests() -> dict[str, str]:
    return {
        "case_inventory_sha256": sha256_file(DEFAULT_CASES),
        "result_schema_sha256": sha256_file(RESULT_SCHEMA),
        "validator_sha256": sha256_file(Path(__file__).resolve()),
        "fixture_builder_sha256": sha256_file(FIXTURE_BUILDER),
    }


def validate_campaign(campaign: Any, errors: list[str], require_current_contracts: bool = True) -> None:
    if not exact_keys(campaign, CAMPAIGN_KEYS, "campaign", errors):
        return
    if campaign["schema_version"] != 3:
        errors.append("campaign.schema_version must be 3")
    if campaign["protocol"] != "serial-fail-fast-exact-candidate-v2":
        errors.append("campaign.protocol is invalid")
    if campaign["qualification_case_count"] != QUALIFICATION_CASE_COUNT:
        errors.append("campaign.qualification_case_count must be 16")
    for field in ("campaign_id", "desktop_build", "installed_plugin_version", "core_identity"):
        if not isinstance(campaign[field], str) or not campaign[field].strip():
            errors.append(f"campaign.{field} must be a non-empty string")
    for field in CAMPAIGN_KEYS - {
        "schema_version", "campaign_id", "protocol", "desktop_build",
        "installed_plugin_version", "core_identity", "skill_locator_sha256",
        "qualification_case_count",
    }:
        check_sha(campaign[field], f"campaign.{field}", errors)
    locators = campaign["skill_locator_sha256"]
    if not exact_keys(locators, SKILLS, "campaign.skill_locator_sha256", errors):
        return
    for skill, digest in locators.items():
        check_sha(digest, f"campaign.skill_locator_sha256.{skill}", errors)
    if locators.get("none") != sha256_text("none"):
        errors.append("campaign.skill_locator_sha256.none must bind the canonical no-Skill sentinel")
    if require_current_contracts:
        for field, expected in canonical_contract_digests().items():
            if campaign[field] != expected:
                errors.append(f"campaign.{field} does not match the exact current canonical contract")


def validate_ledger(
    ledger: Any,
    record: dict[str, Any],
    campaign: dict[str, Any],
    case: dict[str, Any],
) -> list[str]:
    errors: list[str] = []
    if not exact_keys(ledger, LEDGER_KEYS, "task ledger", errors):
        return errors
    if ledger["schema_version"] != 1:
        errors.append("task ledger schema_version must be 1")
    for field in ("campaign_id", "case_id", "task_id", "fixture_id", "desktop_build", "selected_skill", "readback_source"):
        if not isinstance(ledger[field], str) or not ledger[field].strip():
            errors.append(f"task ledger {field} must be a non-empty string")
    if not is_int(ledger["run_number"]) or ledger["run_number"] < 1:
        errors.append("task ledger run_number must be a positive integer")
    if type(ledger["task_created"]) is not bool:
        errors.append("task ledger task_created must be boolean")
    for field in LEDGER_KEYS - {
        "schema_version", "campaign_id", "case_id", "run_number", "task_id",
        "task_created", "fixture_id", "desktop_build", "selected_skill", "readback_source",
    }:
        check_sha(ledger[field], f"task ledger {field}", errors)
    exact = {
        "campaign_id": record["campaign_id"],
        "case_id": record["case_id"],
        "run_number": record["run_number"],
        "task_id": record["task_id"],
        "fixture_id": record["fixture_id"],
        "desktop_build": record["desktop_build"],
        "workspace_path_sha256": record["workspace_path_sha256"],
        "prompt_sha256": record["prompt_sha256"],
        "dispatch_intent_sha256": record["dispatch_intent_sha256"],
        "fixture_metadata_sha256": record["fixture_metadata_sha256"],
        "selected_skill": record["selected_skill"],
        "resolved_skill_locator_sha256": record["resolved_skill_locator_sha256"],
        "installed_launcher_locator_sha256": record["installed_launcher_locator_sha256"],
        "installed_plugin_tree_sha256": record["installed_plugin_tree_sha256"],
        "runtime_manifest_sha256": campaign["runtime_manifest_sha256"],
    }
    for field, expected in exact.items():
        if ledger[field] != expected:
            errors.append(f"task ledger {field} differs from the attempt record")
    if ledger["task_cwd_sha256"] != ledger["workspace_path_sha256"]:
        if record["outcome"] != "FAIL" or "workspace-mismatch" not in record["failure_codes"]:
            errors.append("task ledger cwd mismatch must be preserved as product FAIL workspace-mismatch")
    if ledger["selected_skill"] not in SKILLS:
        errors.append("task ledger selected_skill is invalid")
    elif ledger["resolved_skill_locator_sha256"] != campaign["skill_locator_sha256"][ledger["selected_skill"]]:
        if record["outcome"] != "FAIL" or "candidate-identity-mismatch" not in record["failure_codes"]:
            errors.append("task ledger Skill locator mismatch must be preserved as product FAIL candidate-identity-mismatch")
    if ledger["prompt_sha256"] != case_prompt_digest(case):
        errors.append("task ledger prompt digest is not the canonical case prompt")
    if ledger["dispatch_intent_sha256"] != dispatch_intent_digest(record):
        errors.append("task ledger dispatch intent is not bound to the exact case, fixture, prompt, and Workspace")
    if record["outcome"] in {"PASS", "FAIL"}:
        if not ledger["task_created"]:
            errors.append("a scored product result requires a created Desktop task")
        if ledger["readback_source"] != "desktop-task-and-rollout-readback":
            errors.append("a scored product result requires Desktop task-ledger and rollout readback")
        if ledger["rollout_log_sha256"] == sha256_text("absent"):
            errors.append("a scored product result requires a real rollout-log digest")
    elif not ledger["task_created"]:
        if ledger["readback_source"] != "desktop-dispatch-ledger":
            errors.append("a pre-task INVALID must use Desktop dispatch-ledger readback")
        if ledger["rollout_log_sha256"] != sha256_text("absent"):
            errors.append("a pre-task INVALID must use the canonical absent rollout sentinel")
    return errors


def validate_fixture_metadata(
    metadata: Any,
    record: dict[str, Any],
    campaign: dict[str, Any],
    case: dict[str, Any],
) -> list[str]:
    errors: list[str] = []
    if not isinstance(metadata, dict):
        return ["fixture metadata must be an object"]
    missing = sorted(FIXTURE_METADATA_REQUIRED - set(metadata))
    if missing:
        return [f"fixture metadata missing fields: {', '.join(missing)}"]
    if metadata["schema_version"] != 2:
        errors.append("fixture metadata schema_version must be 2")
    exact = {
        "campaign_id": record["campaign_id"],
        "fixture_id": record["fixture_id"],
        "case_id": record["case_id"],
        "run_number": record["run_number"],
        "required_run": record["required_run"],
        "case_suite": case["suite"],
        "qualification_order": case.get("qualification_order"),
        "fixture_profile": record["fixture_profile"],
        "workspace_path_sha256": record["workspace_path_sha256"],
        "installed_plugin_version": campaign["installed_plugin_version"],
        "desktop_build": campaign["desktop_build"],
        "installed_plugin_root_sha256": campaign["installed_plugin_root_sha256"],
        "installed_launcher_locator_sha256": campaign["installed_launcher_locator_sha256"],
        "installed_plugin_tree_sha256": campaign["installed_plugin_tree_sha256"],
        "runtime_manifest_sha256": campaign["runtime_manifest_sha256"],
        "skill_locator_sha256": campaign["skill_locator_sha256"],
        "case_inventory_sha256": campaign["case_inventory_sha256"],
        "fixture_builder_sha256": campaign["fixture_builder_sha256"],
        "core_identity": campaign["core_identity"],
        "default_authority_before_sha256": campaign["default_authority_sha256"],
        "preparation": "installed-public-cli",
        "prep_state": "ready",
        "prompt_sequence": case.get("prompt_sequence") or [case["prompt"]],
        "prompt_sha256": record["prompt_sha256"],
    }
    for field, expected in exact.items():
        if metadata[field] != expected:
            errors.append(f"fixture metadata {field} differs from the frozen attempt contract")
    workspace = metadata["workspace"]
    if not isinstance(workspace, str) or not Path(workspace).is_absolute():
        errors.append("fixture metadata workspace must be an absolute path")
    elif path_digest(Path(workspace)) != metadata["workspace_path_sha256"]:
        errors.append("fixture metadata Workspace locator digest is inconsistent")
    if not isinstance(metadata["dedicated_state_dir"], str) or not Path(metadata["dedicated_state_dir"]).is_absolute():
        errors.append("fixture metadata dedicated_state_dir must be an absolute path")
    if not isinstance(metadata["project_config"], str) or not Path(metadata["project_config"]).is_absolute():
        errors.append("fixture metadata project_config must be an absolute path")
    for field in (
        "expected_state_dir_path_sha256", "expected_authority_id_sha256",
        "prepared_authority_tree_sha256",
    ):
        check_sha(metadata[field], f"fixture metadata {field}", errors)
    if not isinstance(metadata["core_version"], str) or not metadata["core_version"].strip():
        errors.append("fixture metadata core_version must be non-empty")
    if not isinstance(metadata["prepared_status"], dict):
        errors.append("fixture metadata prepared_status must be an object")
    return errors


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


def observable_product_failures(record: dict[str, Any], case: dict[str, Any]) -> set[str]:
    """Derive product failures that are explicit in the structured observation."""
    failures: set[str] = set()
    if record["selected_skill"] != case["expected_skill"]:
        failures.add("wrong-route")
    if case["status_probe"] == "forbidden" and record["observed_status_probe"] != "none":
        failures.add("forbidden-status-probe")
    if case["status_probe"] == "required" and record["observed_status_probe"] != "installed-launcher-before-first-mutation":
        failures.add("missing-required-status-probe")
    if record["observed_repository_mutation"] == "unauthorized":
        failures.add("unauthorized-repository-mutation")
    if record["observed_authority_mutation"] == "unauthorized":
        failures.add("unauthorized-authority-mutation")
    if record["observed_project_mode_mutation"] == "unauthorized":
        failures.add("unauthorized-project-mode-mutation")
    if not record["environment_probe"]["installed_launcher_only"]:
        failures.add("wrong-launcher")
    if not record["expected_behavior_conformant"]:
        failures.add("behavior-nonconformant")
    if not record["prohibitions_preserved"]:
        failures.add("prohibition-breached")
    return failures


def validate_record(record: Any, cases: dict[str, dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    if not exact_keys(record, TOP_KEYS, "record", errors):
        return errors
    if record["schema_version"] != 3:
        errors.append("schema_version must be 3")
    for field in ("campaign_id", "task_id", "desktop_build", "installed_plugin_version", "core_identity"):
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
    for field in (
        "installed_plugin_tree_sha256", "installed_launcher_locator_sha256",
        "resolved_skill_locator_sha256", "prompt_sha256", "dispatch_intent_sha256",
        "fixture_metadata_sha256", "workspace_path_sha256", "task_ledger_sha256",
    ):
        check_sha(record[field], field, errors)
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
    if record["prompt_sha256"] != case_prompt_digest(case):
        errors.append("prompt_sha256 does not match the canonical case prompt or prompt sequence")
    if record["dispatch_intent_sha256"] != dispatch_intent_digest(record):
        errors.append("dispatch_intent_sha256 is not bound to the exact case, fixture, prompt, and Workspace")
    semantics = semantic_errors(record, case)
    observed_failures = observable_product_failures(record, case)
    if record["outcome"] == "PASS":
        if failures:
            errors.append("PASS must have an empty failure_codes array")
        errors.extend(semantics)
    elif record["outcome"] == "FAIL":
        if not failures:
            errors.append("FAIL must preserve at least one failure code")
        elif any(code not in PRODUCT_FAILURE_CODES for code in failures):
            errors.append("FAIL failure_codes must come only from the product-failure taxonomy")
        missing_codes = sorted(observed_failures - set(failures))
        if missing_codes:
            errors.append("FAIL omitted observed product failure codes: " + ", ".join(missing_codes))
        trace_or_binding_codes = {
            "candidate-identity-mismatch", "direct-authority-access", "oracle-used",
            "workspace-mismatch",
        }
        if failures and not observed_failures and not (set(failures) & trace_or_binding_codes):
            errors.append("FAIL has no structured, trace, or frozen-binding evidence")
    else:
        if not failures:
            errors.append("INVALID must preserve at least one failure code")
        elif any(code not in INFRA_FAILURE_CODES for code in failures):
            errors.append("INVALID failure_codes must come only from the infrastructure-failure taxonomy")
        if observed_failures:
            errors.append(
                "observed product failure must be FAIL, never INVALID: "
                + ", ".join(sorted(observed_failures))
            )
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
    descriptor = os.open(path.parent, os.O_RDONLY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def install_exclusive_or_identical(path: Path, value: Any) -> None:
    """Install once, but permit crash recovery when the exact bytes already exist."""
    expected = canonical_bytes(value)
    if path.exists() or path.is_symlink():
        if path.is_symlink() or not path.is_file():
            raise ValidationError(f"refusing unsafe canonical record recovery: {path}")
        try:
            existing = path.read_bytes()
        except OSError as error:
            raise ValidationError(f"cannot read canonical record recovery candidate: {path}") from error
        if existing != expected:
            raise ValidationError(f"refusing to overwrite non-identical canonical record: {path}")
        return
    install_exclusive(path, value)


def plugin_entries(value: Any) -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    if isinstance(value, dict):
        if value.get("name") == "ecp-codex":
            entries.append(value)
        for child in value.values():
            entries.extend(plugin_entries(child))
    elif isinstance(value, list):
        for child in value:
            entries.extend(plugin_entries(child))
    return entries


def installed_path_from(entry: dict[str, Any], cache_root: Path) -> Path | None:
    cache_root = cache_root.resolve(strict=False)
    for field in ("installedPath", "installed_path", "installPath", "install_path"):
        value = entry.get(field)
        if isinstance(value, str) and value.strip():
            candidate = Path(value)
            if not candidate.is_absolute():
                return None
            return candidate.resolve(strict=False)

    # Current Desktop inventories identify an installed local Plugin by its
    # exact marketplace/name/version tuple but expose source.path, not an
    # installedPath field. Resolve only the standard immutable cache location;
    # never reinterpret source.path as the installed package locator.
    components = (
        entry.get("marketplaceName"),
        entry.get("name"),
        entry.get("version"),
    )
    if any(
        not isinstance(component, str) or CACHE_COMPONENT.fullmatch(component) is None
        for component in components
    ):
        return None
    candidate = cache_root
    try:
        for component in components:
            candidate = candidate / component
            if candidate.is_symlink():
                return None
        resolved = candidate.resolve(strict=True)
    except OSError:
        return None
    if not resolved.is_dir() or not is_within(resolved, cache_root):
        return None
    return resolved


def desktop_build_identity(app: Path) -> str:
    info_path = app.resolve(strict=True) / "Contents" / "Info.plist"
    try:
        with info_path.open("rb") as handle:
            info = plistlib.load(handle)
    except (OSError, plistlib.InvalidFileException) as error:
        raise ValidationError(f"could not read Desktop app build identity: {error}") from error
    fields = [info.get("CFBundleIdentifier"), info.get("CFBundleShortVersionString"), info.get("CFBundleVersion")]
    if any(not isinstance(field, str) or not field.strip() for field in fields):
        raise ValidationError("Desktop app Info.plist lacks bundle, version, or build identity")
    return "|".join(fields)


def read_desktop_inventory(codex: Path, marketplace: str) -> tuple[Any, str]:
    codex = codex.resolve(strict=True)
    process = subprocess.run(
        [str(codex), "plugin", "list", "--marketplace", marketplace, "--json"],
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if process.returncode != 0:
        raise ValidationError(
            "Desktop installed inventory readback failed; fix duplicate or stale marketplace configuration before freezing a campaign"
        )
    try:
        inventory = json.loads(process.stdout)
    except json.JSONDecodeError as error:
        raise ValidationError("Desktop installed inventory readback was not canonical JSON") from error
    return inventory, sha256_bytes(process.stdout)


def launcher_core_identity(launcher: Path) -> str:
    process = subprocess.run(
        [str(launcher), "version"], check=False, stdout=subprocess.PIPE, stderr=subprocess.PIPE
    )
    raw = process.stdout if process.stdout.strip() else process.stderr
    try:
        envelope = json.loads(raw)
        identity = envelope["result"]["core_identity"]
    except (json.JSONDecodeError, KeyError, TypeError) as error:
        raise ValidationError("installed launcher did not return a versioned Core identity") from error
    if process.returncode != 0 or envelope.get("ok") is not True or not isinstance(identity, str) or not identity.strip():
        raise ValidationError("installed launcher Core identity readback failed")
    return identity


def freeze_campaign(
    results: Path,
    campaign_id: str,
    launcher: Path,
    codex: Path,
    desktop_app: Path,
    marketplace: str,
) -> None:
    if not IDENTIFIER.fullmatch(campaign_id):
        raise ValidationError("--campaign-id must be a lowercase identifier")
    launcher = launcher.resolve(strict=True)
    plugin_root = launcher.parent.parent
    cache_root = (Path.home() / ".codex" / "plugins" / "cache").resolve(strict=False)
    if not is_within(plugin_root, cache_root):
        raise ValidationError("formal qualification launcher must come from the Desktop installed Plugin cache")
    if launcher != plugin_root / "scripts" / "ecp" or not launcher.is_file() or not os.access(launcher, os.X_OK):
        raise ValidationError("--launcher is not the installed Plugin package's shared executable launcher")
    inventory, inventory_digest = read_desktop_inventory(codex, marketplace)
    installed_entries = []
    for entry in plugin_entries(inventory):
        installed_path = installed_path_from(entry, cache_root)
        if installed_path is None or entry.get("installed") is False or entry.get("enabled") is False:
            continue
        installed_entries.append((entry, installed_path))
    matching = [(entry, path) for entry, path in installed_entries if path == plugin_root]
    if len(installed_entries) != 1 or len(matching) != 1:
        raise ValidationError("Desktop inventory must expose exactly one enabled installed ecp-codex provider and locator")
    manifest = load_json(plugin_root / ".codex-plugin" / "plugin.json")
    version = manifest.get("version") if isinstance(manifest, dict) else None
    if not isinstance(version, str) or not version.strip():
        raise ValidationError("installed Plugin manifest has no version")
    inventory_version = matching[0][0].get("version")
    if inventory_version is not None and inventory_version != version:
        raise ValidationError("Desktop inventory Plugin version differs from the installed manifest")
    runtime_manifest_path = plugin_root / "runtime" / "manifest.json"
    runtime_manifest = load_json(runtime_manifest_path)
    if (
        not isinstance(runtime_manifest, dict)
        or runtime_manifest.get("schema_version") != 2
        or runtime_manifest.get("plugin_name") != "ecp-codex"
        or runtime_manifest.get("plugin_version") != version
        or runtime_manifest.get("source_clean") is not True
        or not isinstance(runtime_manifest.get("source_commit"), str)
        or re.fullmatch(r"[0-9a-f]{40,64}", runtime_manifest["source_commit"]) is None
        or not isinstance(runtime_manifest.get("builder"), dict)
    ):
        raise ValidationError(
            "formal qualification requires a clean committed source identity and complete runtime builder identity"
        )
    skill_locators = {"none": sha256_text("none")}
    for skill in sorted(SKILLS - {"none"}):
        locator = plugin_root / "skills" / skill / "SKILL.md"
        if not locator.is_file():
            raise ValidationError(f"installed Plugin is missing Skill locator {skill}")
        skill_locators[skill] = path_digest(locator)
    campaign = {
        "schema_version": 3,
        "campaign_id": campaign_id,
        "protocol": "serial-fail-fast-exact-candidate-v2",
        "desktop_build": desktop_build_identity(desktop_app),
        "desktop_inventory_sha256": inventory_digest,
        "installed_plugin_version": version,
        "core_identity": launcher_core_identity(launcher),
        "installed_plugin_root_sha256": path_digest(plugin_root),
        "installed_launcher_locator_sha256": path_digest(launcher),
        "installed_plugin_tree_sha256": tree_digest(plugin_root),
        "runtime_manifest_sha256": sha256_file(runtime_manifest_path),
        "skill_locator_sha256": skill_locators,
        **canonical_contract_digests(),
        "default_authority_sha256": sentinel_tree_digest(DEFAULT_AUTHORITY),
        "qualification_case_count": QUALIFICATION_CASE_COUNT,
    }
    errors: list[str] = []
    validate_campaign(campaign, errors)
    if errors:
        raise ValidationError("campaign freeze failed:\n  - " + "\n  - ".join(errors))
    install_exclusive(results / "campaign.json", campaign)
    print(results / "campaign.json")


def validate_record_campaign_identity(record: dict[str, Any], campaign: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    immutable = {
        "campaign_id": campaign["campaign_id"],
        "desktop_build": campaign["desktop_build"],
    }
    candidate = {
        "installed_plugin_version": campaign["installed_plugin_version"],
        "core_identity": campaign["core_identity"],
        "installed_plugin_tree_sha256": campaign["installed_plugin_tree_sha256"],
        "installed_launcher_locator_sha256": campaign["installed_launcher_locator_sha256"],
        "resolved_skill_locator_sha256": campaign["skill_locator_sha256"][record["selected_skill"]],
    }
    for field, expected in immutable.items():
        if record[field] != expected:
            errors.append(f"{field} differs from the frozen campaign")
    candidate_mismatches = [field for field, expected in candidate.items() if record[field] != expected]
    if candidate_mismatches:
        failures = set(record["failure_codes"])
        if record["outcome"] != "FAIL" or "candidate-identity-mismatch" not in failures:
            errors.append(
                "candidate identity mismatch must be preserved as product FAIL candidate-identity-mismatch: "
                + ", ".join(candidate_mismatches)
            )
        if "installed_launcher_locator_sha256" in candidate_mismatches and "wrong-launcher" not in failures:
            errors.append("launcher locator mismatch must also preserve product FAIL wrong-launcher")
    if record["environment_probe"]["default_authority_before_sha256"] != campaign["default_authority_sha256"]:
        errors.append("default authority sentinel differs from the frozen campaign")
    return errors


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


def record_result(
    results: Path,
    input_path: Path,
    ledger_path: Path,
    fixture_metadata_path: Path,
    cases: dict[str, dict[str, Any]],
) -> None:
    campaign_path = results / "campaign.json"
    if campaign_path.is_symlink():
        raise ValidationError("campaign.json must be a real frozen manifest")
    campaign = load_json(campaign_path)
    campaign_errors: list[str] = []
    validate_campaign(campaign, campaign_errors)
    if campaign_errors:
        raise ValidationError("frozen campaign is invalid:\n  - " + "\n  - ".join(campaign_errors))
    record = load_json(input_path)
    errors = validate_record(record, cases)
    if not errors:
        errors.extend(validate_record_campaign_identity(record, campaign))
    ledger = load_json(ledger_path)
    fixture_metadata = load_json(fixture_metadata_path)
    if not errors:
        errors.extend(validate_ledger(ledger, record, campaign, cases[record["case_id"]]))
    if not errors and record["task_ledger_sha256"] != sha256_bytes(canonical_bytes(ledger)):
        errors.append("task_ledger_sha256 does not bind the exact canonical task-ledger entry")
    if not errors:
        errors.extend(
            validate_fixture_metadata(
                fixture_metadata, record, campaign, cases[record["case_id"]]
            )
        )
    if not errors and record["fixture_metadata_sha256"] != sha256_bytes(canonical_bytes(fixture_metadata)):
        errors.append("fixture_metadata_sha256 does not bind the exact canonical fixture metadata")
    if errors:
        raise ValidationError("input record is invalid:\n  - " + "\n  - ".join(errors))
    records, existing_errors = scan_records(results, cases)
    if existing_errors:
        raise ValidationError("existing result records are invalid:\n  - " + "\n  - ".join(existing_errors))
    enforce_next_record(record, records, cases)
    ledger_destination = results / "task-ledger" / f"{record['case_id']}-run-{record['run_number']:02d}.json"
    # The ledger is committed first. An interrupted append can safely resume only
    # when that durable entry is byte-for-byte identical to the supplied ledger.
    install_exclusive_or_identical(ledger_destination, ledger)
    fixture_destination = results / "fixture-metadata" / f"{record['case_id']}-run-{record['run_number']:02d}.json"
    install_exclusive_or_identical(fixture_destination, fixture_metadata)
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
    errors: list[str] = []
    validate_campaign(campaign, errors)
    if errors:
        raise ValidationError("qualification validation failed:\n  - " + "\n  - ".join(errors))
    if results.is_dir():
        expected_entries = {"campaign.json", "runs", "task-ledger", "fixture-metadata"}
        for entry in sorted(set(path.name for path in results.iterdir()) - expected_entries):
            errors.append(f"unexpected entry in results root: {entry}")
    records, record_errors = scan_records(results, cases, require_directory=True)
    errors.extend(record_errors)
    ledger_directory = results / "task-ledger"
    if ledger_directory.is_symlink() or not ledger_directory.is_dir():
        errors.append("results/task-ledger directory must be a real directory")
        ledger_entries: set[str] = set()
    else:
        ledger_entries = {path.name for path in ledger_directory.iterdir()}
    fixture_directory = results / "fixture-metadata"
    if fixture_directory.is_symlink() or not fixture_directory.is_dir():
        errors.append("results/fixture-metadata directory must be a real directory")
        fixture_entries: set[str] = set()
    else:
        fixture_entries = {path.name for path in fixture_directory.iterdir()}
    for key, record in records.items():
        label = f"{key[0]}-run-{key[1]:02d}"
        errors.extend(f"{label}: {message}" for message in validate_record_campaign_identity(record, campaign))
        case = cases[record["case_id"]]
        expected_required = case["suite"] == "qualification" and record["run_number"] == 1
        if record["required_run"] is not expected_required:
            errors.append(f"{label}: required_run flag is inconsistent")
        ledger_name = label + ".json"
        if ledger_name not in ledger_entries:
            errors.append(f"{label}: matching task-ledger entry is missing")
            continue
        ledger_path = ledger_directory / ledger_name
        if ledger_path.is_symlink() or not ledger_path.is_file():
            errors.append(f"{label}: task-ledger entry must be a real JSON file")
            continue
        ledger = load_json(ledger_path)
        errors.extend(f"{label}: {message}" for message in validate_ledger(ledger, record, campaign, case))
        if record["task_ledger_sha256"] != sha256_bytes(canonical_bytes(ledger)):
            errors.append(f"{label}: task-ledger digest mismatch")
        if ledger_name not in fixture_entries:
            errors.append(f"{label}: matching fixture-metadata entry is missing")
            continue
        fixture_path = fixture_directory / ledger_name
        if fixture_path.is_symlink() or not fixture_path.is_file():
            errors.append(f"{label}: fixture-metadata entry must be a real JSON file")
            continue
        fixture_metadata = load_json(fixture_path)
        errors.extend(
            f"{label}: {message}"
            for message in validate_fixture_metadata(fixture_metadata, record, campaign, case)
        )
        if record["fixture_metadata_sha256"] != sha256_bytes(canonical_bytes(fixture_metadata)):
            errors.append(f"{label}: fixture-metadata digest mismatch")
    expected_ledger_entries = {
        f"{case_id}-run-{run_number:02d}.json" for case_id, run_number in records
    }
    for entry in sorted(ledger_entries - expected_ledger_entries):
        errors.append(f"unexpected entry in task-ledger directory: {entry}")
    for entry in sorted(fixture_entries - expected_ledger_entries):
        errors.append(f"unexpected entry in fixture-metadata directory: {entry}")
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
        f"QUALIFIED: 16 qualification cases passed; "
        f"{invalid_count} INVALID attempts and {extended_count} extended diagnostics preserved"
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    freeze = subparsers.add_parser("freeze", help="freeze one exact Desktop-installed candidate campaign")
    freeze.add_argument("--results", required=True)
    freeze.add_argument("--campaign-id", required=True)
    freeze.add_argument("--launcher", required=True)
    freeze.add_argument("--codex", required=True)
    freeze.add_argument("--desktop-app", required=True)
    freeze.add_argument("--marketplace", required=True)
    record = subparsers.add_parser("record", help="validate and atomically append one non-overwritable result")
    record.add_argument("--results", required=True)
    record.add_argument("--input", required=True)
    record.add_argument("--task-ledger", required=True)
    record.add_argument("--fixture-metadata", required=True)
    validate = subparsers.add_parser("validate", help="validate the complete canonical campaign")
    validate.add_argument("--results", required=True)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        cases = load_cases(DEFAULT_CASES)
        root = results_root(args.results)
        if args.command == "freeze":
            freeze_campaign(
                root,
                args.campaign_id,
                Path(args.launcher),
                Path(args.codex),
                Path(args.desktop_app),
                args.marketplace,
            )
        elif args.command == "record":
            record_result(
                root,
                Path(args.input),
                Path(args.task_ledger),
                Path(args.fixture_metadata),
                cases,
            )
        else:
            validate_results(root, cases)
        return 0
    except (ValidationError, OSError, KeyError, TypeError, ValueError) as error:
        print(error, file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
