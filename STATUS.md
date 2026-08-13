# ECP Current Status

Canonical status as of 2026-08-13. This file answers “where are we and what is
next”; detailed contracts remain in the linked documents.

## Verdict

ECP v0.3 has a local Core/CLI, a four-Skill self-contained Codex Plugin source,
and automated repository-level coverage for the bounded local assurance
lifecycle. The exact clean candidate described below is **qualified for a
controlled real-product pilot through Codex Desktop**. It is not yet a public
release or a claim of production effectiveness: signing, public distribution,
protected CI enforcement, stronger process isolation, and longitudinal
real-project value remain unverified.

ECP's own source Workspace remains unregistered and disabled. ECP itself was
not enabled, used as a qualification fixture, or counted as a real-project
pilot track.

## Current checkpoint

- The formal Plugin version is `0.3.0-dev+codex.20260813115751`. Its runtime
  manifest records `source_commit`
  `66cf40f422661d98d4cd139d54364f04c4896545` and `source_clean: true`. The
  active Darwin/arm64 Core identity is
  `0.3.0-dev+sha256:838212455e6c97c567fef748dd4eb8da95ed268756be9bd411590e13744dd0af`.
- The previous ECP marketplace/provider entries were removed. The candidate is
  installed from the sole remaining ECP provider/cache entry, its installed
  tree matches the packaged source, all Plugin/Skill validators pass using an
  isolated `/private/tmp` PyYAML 6.0.2 target, and Codex Desktop was restarted
  before qualification so fresh tasks loaded this exact installed locator.
- The formal immutable campaign is
  `/Users/o--o/Study/ECPQualificationResults/ecp-codex-20260813-115751`.
  The official validator reports: `QUALIFIED: 16 qualification cases passed;
  0 INVALID attempts and 0 extended diagnostics preserved`.
- All 16 qualification canaries ran serially in distinct fresh Codex Desktop
  tasks, each with a unique opaque Git Workspace and dedicated authority. The
  campaign covered direct, indirect, negative, incomplete, multi-turn,
  disabled, blocked, enable, disable, governed-change, and forbidden-external-
  action paths. The final governed case completed one bounded Change, one
  offline GateRun, one PASS Evidence, and Change completion while preserving
  the prohibition on commit, push, deploy, and CI mutation.
- The source candidate stops greenfield enablement when Core reports `READY`,
  leaves Truth onboarding as a separately authorized later Change, completes
  the initial bootstrap-to-accepted-candidate transaction without an unrelated
  second confirmation, and treats an ambiguous setup request as one
  clarification question with zero repository/ECP probe before confirmation.
  The host contract contains 16 qualification cases and 5 optional
  post-qualification diagnostics.
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
- The four-platform runtime artifacts are packaged and mechanically validated.
  Only the current host's Darwin/arm64 binary was exercised by the Desktop
  qualification; Linux and Darwin/amd64 runtime behavior remains package-level,
  not live-host, evidence.
- Repository tests and static Skill checks remain mechanism evidence. The
  completed Desktop campaign additionally proves that this Desktop build
  selected the intended installed locator, loaded each dedicated authority,
  and followed all 16 natural-language host routes for this exact candidate.
  It does not prove future Desktop builds, other models, remote CI, production,
  or long-running real-project value.
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
- Earlier failed campaigns remain immutable diagnostic evidence for the exact
  candidates that failed. Their failures were fixed in later source commits and
  they were not resumed, merged into, or counted toward the successful formal
  campaign.

## Next actions, in order

1. Use the installed candidate on safe copies of one genuinely new product and
   one established product. Enabling remains a separate explicit whole-
   Workspace decision for each product; Plugin installation alone never enables
   a project.
2. Complete two or three bounded real Changes and one fresh-task handoff in
   each track. Measure recovered context, useful catches, false blockers,
   interaction burden, validation time, and truth-maintenance cost.
3. Decide whether to expand, simplify, or stop the rollout from those
   value-versus-friction observations. Do not pre-commit to a larger matrix or a
   fixed Change count merely to manufacture completion.
4. Only after successful pilot evidence, define a release candidate and add the
   still-required distribution, signing, CI-enforcement, cross-host runtime,
   and operational support gates.

The pilot protocol is [docs/real-project-pilot.md](docs/real-project-pilot.md),
the roadmap is [docs/roadmap.md](docs/roadmap.md), and package boundaries are
[docs/distribution.md](docs/distribution.md).
