# Codex Plugin Distribution

ECP's supported end-user surface is the `ecp-codex` Plugin. The user installs
one Plugin and continues to describe product work in natural language; they do
not install an `ecp` executable, edit PATH, or operate the JSON protocol.

This follows the official Codex packaging model: Skills may contain executable
`scripts/`, repo marketplaces expose local Plugins, and the desktop app loads
an installed copy from its Plugin cache. See the official
[Build skills](https://learn.chatgpt.com/docs/build-skills) and
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

## Trust boundary

The sidecar and runtime manifest detect incomplete copies and accidental or
post-package byte changes. They are stored with the Plugin and are not a
signature: an attacker able to replace the Plugin can replace the launcher,
binary, checksum, and manifest together. The Core binary digest provides exact
Evidence compatibility identity, not publisher provenance.

The current source package is not signed, notarized, published in the universal
directory, or verified in a fresh real desktop task. Those remain release
requirements. Public distribution also needs a supported upgrade/uninstall
policy and a decision about whether Linux packages belong in the same Plugin or
separate platform releases.
