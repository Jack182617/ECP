#!/usr/bin/env python3
"""Build deterministic isolated workspaces for Plugin host qualification.

Every attempt receives a new Git repository and dedicated authority. Enabled
profiles are prepared only through the installed Core's public CLI operations.
The builder never deletes or resets a fixture.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
CASES_PATH = REPO_ROOT / "docs" / "plugin-host-evaluation-cases.json"
SEED_ROOT = REPO_ROOT / "docs" / "plugin-host-fixtures" / "seed"
DEFAULT_AUTHORITY = Path.home() / ".ecp" / "state-v1"
PROFILES = {
    "greenfield-disabled",
    "established-disabled",
    "enabled-clean",
    "enabled-active",
    "enabled-blocked",
}


class FixtureError(RuntimeError):
    pass


def sha256_bytes(value: bytes) -> str:
    return "sha256:" + hashlib.sha256(value).hexdigest()


def tree_digest(root: Path) -> str:
    """Hash an authority tree without emitting its paths or contents."""
    digest = hashlib.sha256()
    if not root.exists() and not root.is_symlink():
        digest.update(b"absent\0")
        return "sha256:" + digest.hexdigest()
    if root.is_symlink() or not root.is_dir():
        raise FixtureError(f"authority sentinel is not a real directory: {root}")
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


def load_cases() -> dict[str, dict[str, Any]]:
    with CASES_PATH.open(encoding="utf-8") as handle:
        inventory = json.load(handle)
    if inventory.get("schema_version") != 3:
        raise FixtureError("fixture builder requires case inventory schema_version 3")
    cases = {case["id"]: case for case in inventory["cases"]}
    if len(cases) != len(inventory["cases"]):
        raise FixtureError("case IDs are not unique")
    return cases


def require_absolute_outside_source(value: str) -> Path:
    path = Path(value)
    if not path.is_absolute():
        raise FixtureError("--batch-root must be absolute")
    resolved = path.resolve(strict=False)
    try:
        resolved.relative_to(REPO_ROOT)
    except ValueError:
        return resolved
    raise FixtureError("fixture batch root must be outside the ECP source repository")


def fixed_git() -> str:
    for candidate in ("/usr/bin/git", "/bin/git"):
        if Path(candidate).is_file() and os.access(candidate, os.X_OK):
            return candidate
    raise FixtureError("no fixed system Git executable is available")


def run_git(workspace: Path, *args: str) -> str:
    environment = os.environ.copy()
    environment.update({"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull})
    process = subprocess.run(
        [fixed_git(), "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgSign=false", *args],
        cwd=workspace,
        env=environment,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if process.returncode != 0:
        raise FixtureError(f"Git command failed: {' '.join(args)}: {process.stderr.strip()}")
    return process.stdout


def run_git_bytes(workspace: Path, *args: str) -> bytes:
    environment = os.environ.copy()
    environment.update({"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull})
    process = subprocess.run(
        [fixed_git(), "-c", "core.hooksPath=/dev/null", *args],
        cwd=workspace,
        env=environment,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if process.returncode != 0:
        raise FixtureError(f"Git command failed: {' '.join(args)}: {process.stderr.decode(errors='replace').strip()}")
    return process.stdout


def source_entry_digest(path: Path) -> str:
    digest = hashlib.sha256()
    if not path.exists() and not path.is_symlink():
        digest.update(b"absent\0")
        return "sha256:" + digest.hexdigest()
    info = path.lstat()
    digest.update(oct(stat.S_IMODE(info.st_mode)).encode() + b"\0")
    if stat.S_ISREG(info.st_mode):
        digest.update(b"file\0")
        with path.open("rb") as handle:
            while chunk := handle.read(1024 * 1024):
                digest.update(chunk)
    elif stat.S_ISLNK(info.st_mode):
        digest.update(b"symlink\0" + os.readlink(path).encode("utf-8") + b"\0")
    else:
        raise FixtureError(f"unsupported changed source entry: {path}")
    return "sha256:" + digest.hexdigest()


def repository_snapshot(workspace: Path) -> dict[str, Any]:
    status = run_git_bytes(workspace, "status", "--porcelain=v1", "-z", "--untracked-files=all")
    tracked_or_untracked = run_git_bytes(workspace, "ls-files", "-co", "--exclude-standard", "-z")
    paths = sorted({part.decode("utf-8") for part in tracked_or_untracked.split(b"\0") if part})
    digest = hashlib.sha256()
    for relative in paths:
        path = workspace / relative
        info = path.lstat()
        digest.update(relative.encode("utf-8") + b"\0" + oct(stat.S_IMODE(info.st_mode)).encode() + b"\0")
        if stat.S_ISREG(info.st_mode):
            digest.update(b"file\0")
            with path.open("rb") as handle:
                while chunk := handle.read(1024 * 1024):
                    digest.update(chunk)
        elif stat.S_ISLNK(info.st_mode):
            digest.update(b"symlink\0" + os.readlink(path).encode("utf-8") + b"\0")
        else:
            raise FixtureError(f"unsupported source-tree entry: {relative}")
    changed = run_git_bytes(workspace, "diff", "--name-only", "-z", "HEAD", "--")
    untracked = run_git_bytes(workspace, "ls-files", "--others", "--exclude-standard", "-z")
    changed_paths = sorted({part.decode("utf-8") for part in (changed + untracked).split(b"\0") if part})
    changed_path_states = [
        {"path": relative, "sha256": source_entry_digest(workspace / relative)}
        for relative in changed_paths
    ]
    return {
        "head": run_git(workspace, "rev-parse", "HEAD").strip(),
        "status_sha256": sha256_bytes(status),
        "source_tree_sha256": "sha256:" + digest.hexdigest(),
        "changed_paths": changed_paths,
        "changed_path_states": changed_path_states,
    }


def toml_string(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def installed_plugin_version(launcher: Path) -> str:
    launcher = launcher.resolve(strict=True)
    try:
        launcher.relative_to(REPO_ROOT)
    except ValueError:
        pass
    else:
        raise FixtureError("qualification preflight must use an installed Plugin cache launcher, not repository source")
    manifest_path = launcher.parent.parent / ".codex-plugin" / "plugin.json"
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        version = manifest["version"]
    except (OSError, json.JSONDecodeError, KeyError, TypeError) as error:
        raise FixtureError("launcher is not inside a readable installed Plugin package") from error
    if not isinstance(version, str) or not version.strip():
        raise FixtureError("installed Plugin manifest has no usable version")
    return version


def invoke_launcher(launcher: Path, authority: Path, workspace: Path, *args: str) -> dict[str, Any]:
    if not launcher.is_absolute() or not launcher.is_file() or not os.access(launcher, os.X_OK):
        raise FixtureError("--launcher must be an absolute executable installed Plugin launcher path")
    environment = os.environ.copy()
    environment["ECP_STATE_DIR"] = str(authority)
    process = subprocess.run(
        [str(launcher), *args],
        cwd=workspace,
        env=environment,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    raw = process.stdout if process.stdout.strip() else process.stderr
    try:
        envelope = json.loads(raw)
    except json.JSONDecodeError as error:
        raise FixtureError(f"installed launcher returned non-JSON output for {' '.join(args)}") from error
    if process.returncode not in (0, 3, 4) or envelope.get("ok") is not True:
        code = (envelope.get("error") or {}).get("code", "UNKNOWN")
        raise FixtureError(f"installed launcher failed for {' '.join(args)}: {code}")
    return envelope


def public_list(value: Any, label: str) -> list[dict[str, Any]]:
    """Normalize a public Core list while rejecting malformed result shapes."""
    if value is None:
        return []
    if not isinstance(value, list) or any(not isinstance(item, dict) for item in value):
        raise FixtureError(f"{label} must be an array of objects or null")
    return value


def project_id(fixture_id: str) -> str:
    suffix = hashlib.sha256(fixture_id.encode()).hexdigest()[:24]
    return f"fixture-{suffix}"


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def prepare_profile(profile: str, workspace: Path, authority: Path, launcher: Path) -> None:
    """Create the requested authority state through public installed-CLI calls."""
    if profile in {"greenfield-disabled", "established-disabled"}:
        return
    inspection = invoke_launcher(
        launcher, authority, workspace, "project", "inspect", "--root", str(workspace)
    )["result"]
    invoke_launcher(
        launcher,
        authority,
        workspace,
        "project",
        "register",
        "--authority",
        inspection["authority_id"],
        "--workspace",
        inspection["workspace_id"],
        "--config-digest",
        inspection["candidate_config_digest"],
        "--truth-digest",
        inspection["candidate_truth_digest"],
        "--actor",
        "fixture-builder",
        "--reason",
        "deterministic disposable fixture registration",
        "--root",
        str(workspace),
    )
    disabled = invoke_launcher(
        launcher, authority, workspace, "project", "status", "--root", str(workspace)
    )["result"]
    invoke_launcher(
        launcher,
        authority,
        workspace,
        "project",
        "enable",
        "--authority",
        disabled["authority_id"],
        "--workspace",
        disabled["workspace_id"],
        "--activation-token",
        disabled["activation_token"],
        "--config-digest",
        disabled["accepted_config_digest"],
        "--truth-digest",
        disabled["accepted_truth_digest"],
        "--actor",
        "fixture-builder",
        "--reason",
        "deterministic disposable fixture enablement",
        "--root",
        str(workspace),
    )

    if profile == "enabled-active":
        current = invoke_launcher(
            launcher, authority, workspace, "context", "get", "--root", str(workspace)
        )["result"]
        acceptance = "Change remains ACTIVE with marker pending and no GateRun or Evidence"
        expected_change = "Maintenance marker changes from pending to ready"
        expected_preservation = "Preparation performs no product write or Gate execution"
        requirement = json.dumps(
            {
                "id": "prepared-high-risk-change",
                "statement": "Keep the high-risk Change active without writing or running a Gate.",
                "status": "DECIDED",
                "verification": "REVIEW",
                "rationale": "The scored disable case requires an exact ACTIVE fixture.",
                "decision_source": "Canonical fixture protocol",
                "covers": {
                    "acceptance_criteria": [acceptance],
                    "expected_changes": [expected_change],
                    "expected_preservations": [expected_preservation],
                },
            },
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        invoke_launcher(
            launcher,
            authority,
            workspace,
            "change",
            "start",
            "--title",
            "Prepared high-risk fixture Change",
            "--goal",
            "Leave one bounded high-risk Change active without product writes",
            "--scope",
            "admin/maintenance.txt",
            "--acceptance",
            acceptance,
            "--risk",
            "high",
            "--impact-component",
            "session-core",
            "--expect-change",
            expected_change,
            "--expect-preserve",
            expected_preservation,
            "--requirement",
            requirement,
            "--authority",
            current["authority_id"],
            "--workspace",
            current["workspace_id"],
            "--activation-token",
            current["activation_token"],
            "--config-digest",
            current["accepted_config_digest"],
            "--truth-digest",
            current["accepted_truth_digest"],
            "--source-fingerprint",
            current["source"]["source_fingerprint"],
            "--root",
            str(workspace),
        )
    elif profile == "enabled-blocked":
        policy_path = workspace / ".ecp" / "policy.json"
        policy = json.loads(policy_path.read_text(encoding="utf-8"))
        if policy.get("max_gate_output_bytes") != 65536:
            raise FixtureError("fixture policy is not at its committed seed value")
        policy["max_gate_output_bytes"] = 65535
        write_json(policy_path, policy)


def build_one(batch_root: Path, case: dict[str, Any], run_number: int, launcher: Path) -> Path:
    if run_number < 1:
        raise FixtureError("run number must be positive")
    fixture_id = f"{case['id']}-run-{run_number:02d}"
    run_root = batch_root / "runs" / fixture_id
    workspace = run_root / "workspace"
    authority = run_root / "authority"
    if run_root.exists() or run_root.is_symlink():
        raise FixtureError(f"refusing to overwrite existing fixture: {run_root}")
    run_root.mkdir(parents=True, mode=0o700)
    authority.mkdir(mode=0o700)
    shutil.copytree(SEED_ROOT / "workspace", workspace, symlinks=True)

    profile = case["fixture_profile"]
    if profile not in PROFILES:
        raise FixtureError(f"unsupported fixture profile: {profile}")
    if profile != "greenfield-disabled":
        shutil.copytree(SEED_ROOT / "project-pack", workspace / ".ecp", symlinks=True)
        project_file = workspace / ".ecp" / "project.json"
        project_file.write_text(
            project_file.read_text(encoding="utf-8").replace("FIXTURE_PROJECT_ID", project_id(fixture_id)),
            encoding="utf-8",
        )

    codex_dir = workspace / ".codex"
    codex_dir.mkdir()
    config = (
        "# Generated fixture contract. The project must be trusted before Codex loads it.\n"
        "[shell_environment_policy]\n"
        'inherit = "core"\n\n'
        "[shell_environment_policy.set]\n"
        f"ECP_STATE_DIR = {toml_string(str(authority))}\n"
    )
    (codex_dir / "config.toml").write_text(config, encoding="utf-8")

    run_git(workspace, "init", "--initial-branch=main")
    run_git(workspace, "add", "--all")
    run_git(
        workspace,
        "-c",
        "user.name=ECP Fixture Builder",
        "-c",
        "user.email=fixture.invalid@example.invalid",
        "commit",
        "-m",
        "Seed disposable plugin-host fixture",
    )
    if profile == "established-disabled":
        note = workspace / "notes" / "operator-note.txt"
        note.write_text(note.read_text(encoding="utf-8") + "unrelated local note\n", encoding="utf-8")

    plugin_version = installed_plugin_version(launcher)
    default_digest = tree_digest(DEFAULT_AUTHORITY)
    empty_authority_digest = tree_digest(authority)
    version = invoke_launcher(launcher, authority, workspace, "version")
    status_envelope = invoke_launcher(launcher, authority, workspace, "project", "status", "--root", str(workspace))
    status = status_envelope["result"]
    if status.get("enabled") is not False or status.get("registered") is not False:
        raise FixtureError("new fixture unexpectedly selected existing authority state")
    expected_pack = profile != "greenfield-disabled"
    if status.get("config_present") is not expected_pack:
        raise FixtureError("installed Core did not load the expected Project Pack shape")
    if tree_digest(authority) != empty_authority_digest:
        raise FixtureError("read-only project status mutated the dedicated authority root")
    if tree_digest(DEFAULT_AUTHORITY) != default_digest:
        raise FixtureError("default authority changed while a dedicated-state preflight ran")

    prepare_profile(profile, workspace, authority, launcher)
    metadata = {
        "schema_version": 1,
        "fixture_id": fixture_id,
        "case_id": case["id"],
        "run_number": run_number,
        "required_run": case["suite"] == "qualification" and run_number == 1,
        "case_suite": case["suite"],
        "qualification_order": case.get("qualification_order"),
        "fixture_profile": profile,
        "workspace": str(workspace),
        "dedicated_state_dir": str(authority),
        "expected_state_dir_path_sha256": sha256_bytes(str(authority).encode()),
        "project_config": str(codex_dir / "config.toml"),
        "expected_authority_id_sha256": sha256_bytes(status["authority_id"].encode()),
        "installed_plugin_version": plugin_version,
        "core_version": version["result"]["core_version"],
        "core_identity": version["result"]["core_identity"],
        "default_authority_before_sha256": default_digest,
        "preparation": "installed-public-cli",
        "prep_state": "ready",
        "prompt_sequence": case.get("prompt_sequence", [case["prompt"]]),
    }
    metadata["prepared_authority_tree_sha256"] = tree_digest(authority)
    write_json(run_root / "fixture.json", metadata)
    verify_prep(str(workspace), launcher)
    return run_root / "fixture.json"


def verify_prep(workspace_value: str, launcher: Path) -> None:
    workspace = Path(workspace_value).resolve(strict=True)
    metadata_path = workspace.parent / "fixture.json"
    metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
    authority = Path(metadata["dedicated_state_dir"])
    expected_config = (
        "# Generated fixture contract. The project must be trusted before Codex loads it.\n"
        "[shell_environment_policy]\n"
        'inherit = "core"\n\n'
        "[shell_environment_policy.set]\n"
        f"ECP_STATE_DIR = {toml_string(str(authority))}\n"
    )
    if (workspace / ".codex" / "config.toml").read_text(encoding="utf-8") != expected_config:
        raise FixtureError("project-scoped Codex state-dir contract drifted")
    version = invoke_launcher(launcher, authority, workspace, "version")["result"]
    if installed_plugin_version(launcher) != metadata["installed_plugin_version"]:
        raise FixtureError("installed Plugin version changed after fixture creation")
    if version["core_identity"] != metadata["core_identity"]:
        raise FixtureError("installed Core identity changed after fixture creation")
    status = invoke_launcher(launcher, authority, workspace, "project", "status", "--root", str(workspace))["result"]
    authority_hash = sha256_bytes(status["authority_id"].encode())
    if authority_hash != metadata["expected_authority_id_sha256"]:
        raise FixtureError("operator status selected a different authority than fixture creation")
    if tree_digest(DEFAULT_AUTHORITY) != metadata["default_authority_before_sha256"]:
        raise FixtureError("default authority changed during fixture preparation")

    profile = metadata["fixture_profile"]
    repository = repository_snapshot(workspace)
    expected_dirty = {
        "greenfield-disabled": [],
        "established-disabled": ["notes/operator-note.txt"],
        "enabled-clean": [],
        "enabled-active": [],
        "enabled-blocked": [".ecp/policy.json"],
    }[profile]
    if repository["changed_paths"] != expected_dirty:
        raise FixtureError(f"{profile} has unexpected prepared repository changes: {repository['changed_paths']}")
    if profile == "enabled-active" and (workspace / "admin" / "maintenance.txt").read_text(encoding="utf-8") != "marker=pending\n":
        raise FixtureError("enabled-active preparation wrote the high-risk product marker before acknowledgement")
    active_change_count = 1 if status.get("active_change", {}).get("state") == "ACTIVE" else 0
    change_count = 0
    gate_run_count = 0
    evidence_count = 0
    if status.get("registered"):
        changes = public_list(
            invoke_launcher(launcher, authority, workspace, "change", "list", "--root", str(workspace))["result"],
            "change list result",
        )
        change_count = len(changes)
        active_change_count = sum(1 for change in changes if change.get("state") == "ACTIVE")
        gate_runs = public_list(
            invoke_launcher(launcher, authority, workspace, "gate", "history", "--root", str(workspace))["result"]["runs"],
            "gate history runs",
        )
        gate_run_count = len(gate_runs)
        for change in changes:
            evidence = public_list(
                invoke_launcher(
                    launcher,
                    authority,
                    workspace,
                    "evidence",
                    "list",
                    "--change",
                    change["change_id"],
                    "--root",
                    str(workspace),
                )["result"]["evidence"],
                "evidence list result",
            )
            evidence_count += len(evidence)
    expected_history = (1, 0, 0) if profile == "enabled-active" else (0, 0, 0)
    if (change_count, gate_run_count, evidence_count) != expected_history:
        raise FixtureError(
            f"{profile} has unexpected Change/GateRun/Evidence counts: "
            f"{change_count}/{gate_run_count}/{evidence_count}"
        )
    if profile in {"greenfield-disabled", "established-disabled"}:
        if status.get("enabled") is not False or status.get("registered") is not False:
            raise FixtureError("disabled fixture preparation unexpectedly mutated authority")
    elif profile == "enabled-clean":
        if status.get("enabled") is not True or status.get("assurance") != "READY" or active_change_count != 0:
            raise FixtureError("enabled-clean requires enabled READY with no ACTIVE Change")
    elif profile == "enabled-active":
        if status.get("enabled") is not True or status.get("assurance") != "ACTIVE" or active_change_count != 1:
            raise FixtureError("enabled-active requires exactly one ACTIVE Change")
    elif profile == "enabled-blocked":
        if status.get("enabled") is not True or status.get("assurance") != "BLOCKED" or active_change_count != 0:
            raise FixtureError("enabled-blocked drift did not produce enabled BLOCKED")
    else:
        raise FixtureError(f"unsupported fixture profile: {profile}")

    metadata["prep_state"] = "ready"
    metadata["prepared_status"] = {
        "registered": status.get("registered"),
        "enabled": status.get("enabled"),
        "assurance": status.get("assurance"),
        "active_change_count": active_change_count,
        "change_count": change_count,
        "gate_run_count": gate_run_count,
        "evidence_count": evidence_count,
    }
    metadata["prepared_authority_tree_sha256"] = tree_digest(authority)
    write_json(metadata_path, metadata)


def public_snapshot(workspace_value: str, launcher: Path) -> dict[str, Any]:
    workspace = Path(workspace_value).resolve(strict=True)
    metadata = json.loads((workspace.parent / "fixture.json").read_text(encoding="utf-8"))
    authority = Path(metadata["dedicated_state_dir"])
    status = invoke_launcher(launcher, authority, workspace, "project", "status", "--root", str(workspace))["result"]
    changes: list[dict[str, Any]] = []
    gate_runs: list[dict[str, Any]] = []
    evidence_count = 0
    if status.get("registered"):
        changes = public_list(
            invoke_launcher(launcher, authority, workspace, "change", "list", "--root", str(workspace))["result"],
            "change list result",
        )
        gate_report = invoke_launcher(launcher, authority, workspace, "gate", "history", "--root", str(workspace))["result"]
        gate_runs = public_list(gate_report["runs"], "gate history runs")
        for change in changes:
            evidence = invoke_launcher(
                launcher, authority, workspace, "evidence", "list", "--change", change["change_id"], "--root", str(workspace)
            )["result"]
            evidence_count += len(public_list(evidence["evidence"], "evidence list result"))
    return {
        "repository": repository_snapshot(workspace),
        "authority": {
            "tree_sha256": tree_digest(authority),
            "registered": bool(status.get("registered")),
            "enabled": bool(status.get("enabled")),
            "assurance": status["assurance"],
            "active_change_count": 1 if status.get("active_change") else 0,
            "active_gate_run_count": 1 if status.get("active_gate_run") else 0,
            "change_count": len(changes),
            "completed_change_count": sum(1 for change in changes if change.get("state") == "COMPLETED"),
            "cancelled_change_count": sum(1 for change in changes if change.get("state") == "CANCELLED"),
            "gate_run_count": len(gate_runs),
            "evidence_count": evidence_count,
        },
    }


def environment_probe(workspace_value: str, launcher: Path) -> dict[str, Any]:
    workspace = Path(workspace_value).resolve(strict=True)
    metadata = json.loads((workspace.parent / "fixture.json").read_text(encoding="utf-8"))
    authority = Path(metadata["dedicated_state_dir"])
    status = invoke_launcher(launcher, authority, workspace, "project", "status", "--root", str(workspace))["result"]
    version = invoke_launcher(launcher, authority, workspace, "version")["result"]
    return {
        "fixture_state_dir_path_sha256": metadata["expected_state_dir_path_sha256"],
        "operator_state_dir_path_sha256": sha256_bytes(str(authority).encode()),
        "fixture_authority_id_sha256": metadata["expected_authority_id_sha256"],
        "operator_authority_id_sha256": sha256_bytes(status["authority_id"].encode()),
        "fixture_plugin_version_sha256": sha256_bytes(metadata["installed_plugin_version"].encode()),
        "operator_plugin_version_sha256": sha256_bytes(installed_plugin_version(launcher).encode()),
        "fixture_core_identity_sha256": sha256_bytes(metadata["core_identity"].encode()),
        "operator_core_identity_sha256": sha256_bytes(version["core_identity"].encode()),
        "default_authority_sha256": tree_digest(DEFAULT_AUTHORITY),
        "installed_plugin_version": installed_plugin_version(launcher),
        "core_identity": version["core_identity"],
    }


def verify_seed(launcher: Path) -> None:
    cases = load_cases()
    representative: dict[str, dict[str, Any]] = {}
    for case in cases.values():
        representative.setdefault(case["fixture_profile"], case)
    if set(representative) != PROFILES:
        raise FixtureError("case inventory does not cover all five fixture profiles")
    with tempfile.TemporaryDirectory(prefix="ecp-plugin-host-seed-") as temporary:
        batch_root = Path(temporary).resolve()
        for profile in sorted(PROFILES):
            build_one(batch_root, representative[profile], 1, launcher)
    print("validated all five generated fixture profiles with the installed launcher")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    create = subparsers.add_parser("create-run", help="create one non-overwritable independent run fixture")
    create.add_argument("--batch-root", required=True)
    create.add_argument("--case-id", required=True)
    create.add_argument("--run-number", required=True, type=int)
    create.add_argument("--launcher", required=True)
    prep = subparsers.add_parser("verify-prep", help="verify and record one fixture's public prepared state")
    prep.add_argument("--workspace", required=True)
    prep.add_argument("--launcher", required=True)
    snapshot = subparsers.add_parser("snapshot", help="print canonical public repository/authority summary JSON")
    snapshot.add_argument("--workspace", required=True)
    snapshot.add_argument("--launcher", required=True)
    environment = subparsers.add_parser("environment-probe", help="print hashed authority/config sentinel values")
    environment.add_argument("--workspace", required=True)
    environment.add_argument("--launcher", required=True)
    verify = subparsers.add_parser("verify-seed", help="Core-load/status preflight all five generated profiles")
    verify.add_argument("--launcher", required=True)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        if args.command == "create-run":
            cases = load_cases()
            if args.case_id not in cases:
                raise FixtureError(f"unknown case ID: {args.case_id}")
            metadata = build_one(
                require_absolute_outside_source(args.batch_root),
                cases[args.case_id],
                args.run_number,
                Path(args.launcher),
            )
            print(metadata)
        elif args.command == "verify-prep":
            verify_prep(args.workspace, Path(args.launcher))
        elif args.command == "snapshot":
            print(json.dumps(public_snapshot(args.workspace, Path(args.launcher)), ensure_ascii=False, indent=2, sort_keys=True))
        elif args.command == "environment-probe":
            print(json.dumps(environment_probe(args.workspace, Path(args.launcher)), ensure_ascii=False, indent=2, sort_keys=True))
        elif args.command == "verify-seed":
            verify_seed(Path(args.launcher))
        return 0
    except (FixtureError, OSError, KeyError, ValueError) as error:
        print(f"fixture error: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
