# ECP Current Status

Canonical status as of 2026-08-13. This file answers “where are we and what is
next”; detailed contracts remain in the linked documents.

## Verdict

ECP v0.3 has a local Core/CLI, a four-Skill self-contained Codex Plugin source,
and automated repository-level coverage for the bounded local assurance
lifecycle. It is **not yet qualified for a real-product pilot or release**.
Signing, public distribution, protected CI enforcement, isolated execution,
and longitudinal real-project value remain unverified.

The previous duplicated host-routing campaign is terminated. Its completed
observations may be retained as diagnostic records, but the campaign and its
disposable Workspaces/authorities do not constitute qualification evidence and
must not be resumed to reach a historical run count. ECP itself is never a host
fixture or a real-project pilot track.

## Current checkpoint

- The source candidate now stops greenfield enablement when Core reports
  `READY`, leaves Truth onboarding as a separately authorized later Change, and
  carries the serial 12-qualification/9-extended host contract. Focused source,
  fixture, validator, Core, and CLI checks pass locally. This is not yet a
  frozen or installed exact candidate.
- Repository tests and static Skill checks remain useful mechanism evidence,
  but they do not prove that Codex Desktop selected the intended installed
  locator, loaded the dedicated authority, or followed the natural-language
  lifecycle in a fresh task.
- The replacement Desktop gate is a **single serial campaign of 12 fail-fast
  host canaries**, one fresh run per canary, against one exact installed Plugin
  version and one exact bundled Core identity. Qualification requires `12/12`
  for that unchanged candidate.
- A canary outcome is either `PASS`, product `FAIL`, or infrastructure
  `INVALID`. The first product `FAIL` stops the campaign and requires a product
  fix, rebuilt exact candidate, and a new campaign. `INVALID` records an
  unusable fixture/host/dispatch observation; it is preserved, does not count
  as a product pass or failure. After correcting the infrastructure cause, the
  same canary may be retried once in a fresh fixture; a second `INVALID` freezes
  that candidate campaign.
- Disposable Workspaces and dedicated authorities remain isolated and may be
  removed after evidence capture. The campaign manifest, task ledger, exact
  identities, observations, scores, failures, invalid runs, and retries must
  live in a durable external results root outside the ECP repository and must
  not use `/tmp` or `/private/tmp` as their only copy.

## Next actions, in order

1. Freeze all intended Core, Skill, reference, protocol, and manifest source at
   one reproducible checkpoint before packaging. The current dirty working tree
   is not that checkpoint; packaging still requires either an explicitly
   authorized local commit or an explicitly revised clean-staging contract.
2. Build and install one new cachebuster candidate. In a fresh read-only task,
   freeze the exact installed Skill locator, Plugin version, bundled Core
   identity, and unchanged default project mode.
3. Execute the 12 canaries once each, serially and fail-fast, using a fresh
   disposable Git Workspace and dedicated `ECP_STATE_DIR` for every canary.
   Preserve every result in the durable external results root and require
   `12/12` for the exact unchanged candidate.
4. Only after that gate passes, use safe copies of one genuinely new product
   and one established product. Complete two or three bounded real Changes and
   one fresh-task handoff in each track. Measure recovered context, useful
   catches, false blockers, interaction burden, validation time, and truth
   maintenance cost.
5. Decide whether to expand, simplify, or stop the rollout from those
   value-versus-friction observations. Do not pre-commit to a larger matrix or a
   fixed Change count merely to manufacture completion.

The pilot protocol is [docs/real-project-pilot.md](docs/real-project-pilot.md),
the roadmap is [docs/roadmap.md](docs/roadmap.md), and package boundaries are
[docs/distribution.md](docs/distribution.md).
