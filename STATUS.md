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
  treats an ambiguous setup request as one clarification question with zero
  repository/ECP probe before confirmation. The host contract contains 16
  qualification cases and 5 post-qualification diagnostics, including the four
  indirect natural-language routing paths.
- Core durability fixes now cover physical repository/authority export aliases,
  crash-safe no-replace Workspace bindings with controlled truncated-remnant
  recovery and exact post-binding bootstrap resumption, fsync-checked
  Evidence/export installation, Gate primary+terminal failure preservation,
  and broken stderr fallback. The canonical Core and CLI suites, repository
  contract tests, Python qualification-protocol tests, Go vet, official Plugin
  validator, and all four Skill validators pass in the current worktree.
- Formal host qualification now freezes exactly one Desktop-installed provider,
  Desktop build/inventory, Plugin tree/launcher/Skill locators, Core identity,
  case/schema/validator/fixture contracts, and default-authority sentinel. Each
  result is bound to the canonical prompt, fixture metadata, exact Workspace,
  Desktop task ledger, resolved Skill locator, and rollout readback. `INVALID`
  uses a closed infrastructure taxonomy and cannot hide an observed product
  failure.
- A new four-platform development artifact has been built and mechanically
  validated, but its runtime manifest correctly records `source_clean: false`.
  It is not a formal candidate and has not been installed or qualified. Any
  earlier installed cache or terminated campaign predates these source changes
  and is diagnostic only. This work did not modify global Codex configuration
  or install a Plugin.
- Repository tests and static Skill checks remain useful mechanism evidence,
  but they do not prove that Codex Desktop selected the intended installed
  locator, loaded the dedicated authority, or followed the natural-language
  lifecycle in a fresh task.
- The replacement Desktop gate is a **single serial campaign of 16 fail-fast
  host canaries**, one fresh run per canary, against one exact installed Plugin
  tree/version, one exact bundled Core identity, and one exact Desktop build.
  Qualification requires `16/16` for that unchanged candidate.
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

1. After separately authorized source commit/clean-epoch, package a new
   cachebuster candidate. After separately authorized installation of the
   Plugin, freeze the single-provider Desktop inventory and validate all five
   deterministic fixture profiles. Never promote the current dirty development
   artifact.
2. Restore a usable Codex Desktop dispatch/readback path, then execute the 16 canaries
   once each, serially and fail-fast, using a fresh
   disposable Git Workspace and dedicated `ECP_STATE_DIR` for every canary.
   Preserve every result in the durable external results root and require
   `16/16` for the exact unchanged candidate. Preserve the immutable campaign,
   fixture metadata, task ledger, rollout digest, and all failed/invalid runs.
3. Only after that gate passes, use safe copies of one genuinely new product
   and one established product. Complete two or three bounded real Changes and
   one fresh-task handoff in each track. Measure recovered context, useful
   catches, false blockers, interaction burden, validation time, and truth
   maintenance cost.
4. Decide whether to expand, simplify, or stop the rollout from those
   value-versus-friction observations. Do not pre-commit to a larger matrix or a
   fixed Change count merely to manufacture completion.

The pilot protocol is [docs/real-project-pilot.md](docs/real-project-pilot.md),
the roadmap is [docs/roadmap.md](docs/roadmap.md), and package boundaries are
[docs/distribution.md](docs/distribution.md).
