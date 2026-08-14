# ECP — Engineering Control Plane

[English](README.md) | [简体中文](README.zh-CN.md)

ECP is a local-first, tool-agnostic engineering control plane for auditable,
evidence-bound AI-assisted software changes.

Coding agents can generate code quickly, but speed alone does not preserve a
project's intent, constraints, decisions, or proof that a change is complete.
ECP keeps those facts with the project and binds each governed change to an
exact source state, accepted project truth, required validation gates, durable
evidence, and a current verdict.

> [!IMPORTANT]
> ECP is an early-stage preview for controlled local pilots. It is not a
> production enforcement system, a public Plugin release, or proof that an AI
> change is defect-free. Read [Current status](STATUS.md) before evaluating or
> adopting it.

## Why ECP

Long-running AI-native development has a continuity problem:

- chat history is temporary and model-specific;
- requirements and architectural boundaries drift across tasks;
- old test results can be mistaken for evidence about new source;
- an agent can overstate completion when validation is incomplete;
- local workflow rules are not the same as protected enforcement.

ECP addresses that problem with a narrow lifecycle:

```text
Project Truth
  -> Change Contract
  -> Impact Analysis
  -> Agent Implementation
  -> Semantic Reconciliation
  -> Required Gates
  -> Exact Evidence
  -> Verdict
```

The goal is not to make AI infallible. The goal is to make known ambiguity,
scope drift, stale evidence, and unsupported completion claims harder to pass
silently.

## What exists today

The repository currently contains:

- a Go Core and CLI implementing workspace-local project mode, bounded Change
  lifecycles, accepted Project Truth, structured requirements and impact,
  explicit accepted-Unknown dispositions, bounded/exact history queries,
  GateRun history, evidence applicability, and derived verdicts;
- a self-contained `ecp-codex` Plugin with four focused Skills for checking,
  enabling, disabling, and governing changes;
- checksum-verified bundled runtimes for Darwin and Linux on arm64 and amd64;
- repository-level Go and Python regression tests;
- a threat model, verification matrix, host-routing qualification protocol,
  real-project pilot protocol, and release boundary documentation;
- a completed 16-case qualification campaign for one exact Codex Desktop
  build, installed Plugin candidate, and bundled Core identity.

That qualification establishes behavior only for the frozen candidate and
host campaign. The current source contains a newer post-qualification
iteration and is not an installed or qualified replacement candidate. Neither
state establishes future-host compatibility, public
distribution provenance, protected CI enforcement, production effectiveness,
or long-term value in real projects.

## Core invariants

- Every canonical Git workspace starts with ECP disabled.
- Installing the Plugin or finding an `.ecp/` directory never enables a
  project implicitly.
- An ambiguous request to configure ECP is clarified before the adapter reads
  repository contents or probes project status.
- An enabled workspace stays enabled when blocked or indeterminate; it does not
  silently fall back to ungoverned editing.
- Verdicts are derived from the current accepted configuration, Project Truth,
  source fingerprint, semantic reconciliation, Gate plan, and applicable
  evidence.
- Local `PASS` means only that the accepted local policy's declared gates
  passed for the exact current subject. It does not mean released, deployed,
  remotely attested, production-tested, or defect-free.
- ECP does not authorize commits, pushes, releases, deployments, production
  access, or other external effects that the user did not authorize.

The complete product direction is in [NORTH_STAR.md](NORTH_STAR.md), while
[SPEC.md](SPEC.md) defines the current executable contract.

## Architecture

```text
User request
    |
    v
Coding agent + ECP adapter Skills
    |
    v
ECP Core / CLI
    |-- repository .ecp/       reviewable candidate policy and Project Truth
    |-- Git workspace          source state under observation
    |-- configured gates       local validation commands
    `-- external state store   activation, history, evidence, and verdict data

Future trusted consumers
    |-- protected CI
    |-- branch rules
    `-- release/deploy verification
```

ECP is a workflow control layer, not a replacement for coding agents, IDEs,
Git, test frameworks, CI, release systems, or production monitoring.

## Repository map

| Path | Purpose |
| --- | --- |
| `cmd/ecp` | CLI entry point |
| `internal/ecp` | Core lifecycle, storage, evidence, truth, and security logic |
| `internal/cli` | Stable JSON command interface |
| `plugins/ecp-codex` | Codex Plugin, four Skills, launcher, and bundled runtimes |
| `scripts` | Packaging and host-qualification tooling |
| `docs` | Architecture, threat model, evaluation, pilot, and distribution contracts |
| `.ecp` | ECP's own candidate Project Pack; it does not self-enable the repository |

## Build and test

Requirements:

- Go 1.24;
- Python 3 for the qualification-protocol tests;
- Git at `/usr/bin/git` or `/bin/git` for authority operations on supported
  POSIX hosts.

Run the focused repository checks:

```bash
go test ./...
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
```

Build the development CLI:

```bash
go build -o bin/ecp ./cmd/ecp
./bin/ecp version
```

The formal Plugin packaging script intentionally requires a clean committed
source state and regenerates checked-in runtime artifacts. Contributors should
not run it for an ordinary pull request unless a maintainer asks for a package
candidate. See [Plugin distribution operations](docs/distribution.md).

## Codex Plugin preview

The repository is structured as a local Codex Plugin marketplace. The Plugin
is a preview for controlled evaluation and is not a signed public release.
Installing it does not enable ECP in any project.

Developer installation and rollback instructions are documented in
[docs/distribution.md](docs/distribution.md). Host-routing evaluation is
defined in [docs/plugin-host-evaluation.md](docs/plugin-host-evaluation.md),
and real-product evaluation is defined in
[docs/real-project-pilot.md](docs/real-project-pilot.md).

## Security model

ECP treats repositories, configured gates, logs, candidate Project Packs, and
adapter output as untrusted inputs. It uses exact source and execution
identities, private local authority files, bounded artifacts, stable reads,
checksum verification, and fail-closed lifecycle states to reduce specific
risks.

These controls are not an OS sandbox, authenticated approval, code signing,
remote attestation, or non-bypassable enforcement. Same-user processes and
tools outside the supported adapter workflow can bypass local controls. See
[docs/security-model.md](docs/security-model.md) for the complete threat model
and [SECURITY.md](SECURITY.md) for vulnerability reporting.

## Project status and roadmap

- [STATUS.md](STATUS.md) — canonical current state and next gate
- [docs/roadmap.md](docs/roadmap.md) — staged product roadmap
- [docs/verification-matrix.md](docs/verification-matrix.md) — verified and
  unverified surfaces
- [CHANGELOG.md](CHANGELOG.md) — user-visible changes

The next engineering gates are repository validation, an explicitly
authorized newly versioned package, installed upgrade/rollback validation, and
a fresh 16-case qualification for that exact candidate. Only then is the next
product gate a bounded pilot on safe copies of one genuinely new project and
one established project. ECP's own source repository is not a fixture or
real-project pilot track.

## Contributing

Issues and pull requests are welcome. Start with
[CONTRIBUTING.md](CONTRIBUTING.md), review the current maturity and non-goals,
and keep proposed changes bounded. Security vulnerabilities should be reported
privately according to [SECURITY.md](SECURITY.md).

## License

ECP is licensed under the [Apache License 2.0](LICENSE). Contributions are
accepted under the same license unless explicitly stated otherwise.

ECP is an independent open-source project. It is not affiliated with or
endorsed by OpenAI.
