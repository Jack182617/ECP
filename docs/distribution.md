# Codex Plugin Distribution

ECP's supported end-user surface is the `ecp-codex` Plugin. The user installs
one Plugin and continues to describe product work in natural language; they do
not install an `ecp` executable, edit PATH, or operate the JSON protocol.

This follows the official Codex packaging model: Skills may contain executable
`scripts/`, repo marketplaces expose local Plugins, and the desktop app loads
an installed copy from its Plugin cache. See the official
[Build skills](https://developers.openai.com/plugins/build/skills) and
[Package your plugin](https://developers.openai.com/plugins/build/plugins)
documentation.

## Package layout

```text
plugins/ecp-codex/
├── .codex-plugin/plugin.json
├── references/
│   ├── cli-contract.md
│   └── project-config.md
├── scripts/ecp
├── skills/
│   ├── ecp-check/{SKILL.md,agents/openai.yaml}
│   ├── ecp-enable/{SKILL.md,agents/openai.yaml}
│   ├── ecp-disable/{SKILL.md,agents/openai.yaml}
│   └── ecp-change/{SKILL.md,agents/openai.yaml}
└── runtime/
    ├── manifest.json
    ├── darwin-arm64/ecp{,.sha256}
    ├── darwin-amd64/ecp{,.sha256}
    ├── linux-arm64/ecp{,.sha256}
    └── linux-amd64/ecp{,.sha256}
```

Each Skill resolves the shared Plugin-root `scripts/ecp` from its own installed
`SKILL.md`. The launcher resolves the Plugin root from that physical path,
chooses only the matching packaged OS/architecture, rejects symlinks or
non-executable runtime files, calculates SHA-256 with a fixed system executable,
compares the sidecar, and only then `exec`s Core. It never resolves an ambient
`ecp` from PATH.

Missing, unsupported, unsafe, or checksum-mismatched runtimes return a
versioned `ECP_RUNTIME_*` error with exit 4 before Core can inspect or mutate a
repository. The runtime then applies Core's independent platform, authority,
Git, config, Change, and Evidence checks.

## Build and version order

The release order is mandatory:

1. Finish Core, Skill, shared reference, and manifest source edits.
2. Bump the Plugin cachebuster/version with the official Plugin Creator helper.
3. Run `./scripts/package-plugin.sh` with a private writable Go cache.
4. Validate `runtime/manifest.json`, every binary digest/size/execute bit, the
   Plugin manifest, all four Skills, and a copied cache-layout launcher smoke test.
5. Sign/notarize and publish through the chosen trusted distribution process.

`package-plugin.sh` builds with CGO disabled, `-trimpath`, and
`-buildvcs=false` for the four supported targets. It stages all artifacts first
and replaces the prior runtime directory only after every build and checksum
succeeds. The generated runtime manifest binds all artifact paths, digests, and
sizes to the current Plugin version.

Changing Plugin source after packaging makes the package stale. Changing the
Plugin version after packaging makes the manifest version mismatch. Both must
fail validation; do not hand-edit generated checksums or the runtime manifest.

## Installation boundary

The repo marketplace at `.agents/plugins/marketplace.json` exposes the Plugin
as `ECP Local`. Opening the repository in the desktop app, restarting it, and
installing from that marketplace are user-visible installation actions. ECP
does not silently edit global Codex configuration or install itself.

Plugin installation is not project enablement. Each canonical Git Workspace
still defaults to ECP disabled and requires an explicit project-level enable
request before authority state is created or governed Changes begin.

## Local Plugin operations

The following policy applies to the source-backed `ecp-local` development
marketplace. It is an operator workflow, not an ordinary project-user workflow,
and it does not authorize publication, project enablement, authority migration,
or deletion.

### Install and accept one local build

1. Require a clean canonical Plugin source at the intended Git commit. Confirm
   that `plugin.json` and `runtime/manifest.json` carry the same new version and
   that every packaged artifact matches its declared digest, size, and execute
   bit.
2. Run the official Plugin validator and Skill validator for all four Skills,
   plus the copied cache-layout launcher tests. Source-tree success alone is
   not installation acceptance.
3. Install or update with `codex plugin add ecp-codex@ecp-local --json`. Treat
   the returned version and `installedPath` as the candidate installation; do
   not infer the active copy from the marketplace source path.
4. Compare the installed copy with the reviewed source, validate the installed
   Plugin and all four installed Skills, and invoke only the installed shared
   launcher for `ecp version` and read-only project status.
5. Restart the desktop app and open a new task. Invoke `$ecp-check` with a short
   read-only request to report project mode, Core version, and full Core
   identity. Success requires the expected installed Plugin version, the exact
   packaged Core identity, and an unchanged project mode. A source/cache check
   in the task that performed installation does not prove fresh-task pickup.
6. Before any real-product pilot write, complete the independent disposable-
   fixture matrix in [Codex Plugin host-routing evaluation](plugin-host-evaluation.md).
   A successful explicit check does not prove enable, change, disable, negative,
   incomplete, or edge-case routing.

### Upgrade

1. Do not upgrade through an unreviewed dirty marketplace source. Finish or
   explicitly cancel ACTIVE Changes when practical; otherwise record that a
   Core identity or compatibility change may invalidate the current plan and
   require new Evidence after upgrade.
2. Apply the mandatory build-and-version order above. Every Core, Skill,
   launcher, reference, or manifest change receives a new cachebuster and a
   complete package validation before installation.
3. Re-run `codex plugin add ecp-codex@ecp-local --json`; never overwrite an
   installed cache directory in place. Restart into a new task and repeat the
   read-only installed-copy acceptance.
4. The upgrade must preserve every Workspace mode and authority history. A new
   Core identity intentionally prevents old plans or Evidence from being
   treated as evidence for a different execution identity. If Core reports an
   unsupported or incompatible authority/config schema, stop fail-closed; do
   not edit the external state store or invent an implicit migration.

### Failure recovery and rollback

- Never point a Skill at an older cache directory, hand-edit cached files, copy
  a binary over the installed runtime, or bypass checksum/compatibility errors
  with an ambient CLI.
- Revert the faulty source change through normal Git review, allocate a new
  cachebuster, rebuild all runtime artifacts, run the full package/Skill
  validation, and install that corrective package as a new version. This is a
  forward-delivered rollback and remains auditable.
- Rollback installation must not change project mode or authority state. If an
  enabled project cannot be inspected with the new package, make no ungoverned
  repository mutation; reinstall a compatible validated package first. Then
  derive a fresh plan and Evidence where the execution identity changed.
- Retaining an older cache directory is not a supported backup or restore
  mechanism. Authority export/verification is separately explicit and ECP
  v0.3 still has no authority restore/import operation.

### Uninstall

1. Before removal, explicitly disable ECP in every enabled Workspace that must
   remain editable without the Plugin. If a Workspace intentionally remains
   enabled, accept that supported Codex repository mutation is unavailable
   until a compatible Plugin is reinstalled; uninstall is not a task-level ECP
   bypass.
2. Remove only through `codex plugin remove ecp-codex@ecp-local --json`, then
   restart and verify the Plugin is absent from the installed list. Removing
   the Plugin does not remove the configured marketplace unless that separate
   action is explicitly requested.
3. Uninstall must not delete or rewrite `.ecp`, external authority state,
   accepted Project Truth, Change history, Evidence, or project source. It must
   not be reported as project disablement or history erasure.

## Trust boundary

The sidecar and runtime manifest detect incomplete copies and accidental or
post-package byte changes. They are stored with the Plugin and are not a
signature: an attacker able to replace the Plugin can replace the launcher,
binary, checksum, and manifest together. The Core binary digest provides exact
Evidence compatibility identity, not publisher provenance.

The current installed-copy read-only smoke is recorded in
`docs/plugin-host-evaluation.md`: the reviewed source and installed cache files
matched, the installed launcher returned the expected Core identity, and an
explicit check preserved disabled project mode and clean Git state. That task
carried prior context, so it is not evidence for the independent fresh-task
matrix.

The current source package is still unsigned, unnotarized, and unpublished in
the universal directory. Fresh-task enable/change/disable routing, upgrade,
uninstall, reinstallation, and a governed Change have not been validated. Keep
these independent claims separate: read-only installed-copy discovery is
observed, while host routing, lifecycle compatibility, and publisher provenance
remain unverified release requirements. A separate release decision is also
required for keeping Linux packages in the same Plugin versus platform-specific
distribution.
