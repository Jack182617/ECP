# Security Policy

ECP is an early-stage local engineering control plane. Security reports are
welcome, especially when they concern authority integrity, path containment,
source/evidence binding, command execution, Plugin runtime verification,
cross-workspace isolation, or a claim that exceeds the documented trust model.

## Supported versions

ECP does not yet have a stable public release. Security fixes are developed on
the `main` branch. Frozen qualification candidates are historical evidence for
their exact source and host identities; they are not maintained release lines.

## Report a vulnerability privately

Use GitHub's private vulnerability reporting flow:

<https://github.com/Jack182617/ECP/security/advisories/new>

Do not open a public issue for an unpatched vulnerability. Include only the
minimum information needed to reproduce and assess the report:

- affected commit, Plugin version, Core identity, platform, and architecture;
- preconditions and trust boundary involved;
- a minimal reproduction or proof of concept;
- expected versus observed behavior;
- impact and whether credentials, personal data, or external systems were
  involved;
- suggested mitigation, if known.

Redact tokens, credentials, private repository content, authority data,
Evidence logs, and personal paths. Do not test repositories, accounts, or
systems you do not own or lack permission to assess.

The maintainer will acknowledge actionable reports when practical, coordinate
validation and remediation, and publish an advisory when disclosure is safe.
No fixed response or release SLA is promised during the preview stage.

## Security boundaries that are not vulnerabilities by themselves

The following are documented limits of the current design:

- ECP is not an OS sandbox and runs local gates as the current user.
- The supported Codex adapter workflow can be bypassed by other tools, direct
  shell access, Plugin disablement, or same-user processes.
- Local acknowledgement is not authenticated human approval.
- Checksums provide package consistency, not signing or provenance.
- Local `PASS` is not release, deployment, production, or remote-attestation
  evidence.
- v0.3 does not provide authority restore/import, encryption, universal secret
  redaction, or protected CI enforcement.

Reports that show a documented boundary can be crossed in a way the current
threat model claims to prevent are in scope. The complete model is in
[docs/security-model.md](docs/security-model.md).
