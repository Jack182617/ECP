# ECP Current Status

Canonical status as of 2026-08-14. This file answers “where are we and what is
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

The current installed candidate is now the only candidate accepted for the next
controlled pilot decision. It has passed independent fresh-task installed-copy
acceptance, an isolated installed-Core upgrade lifecycle, and a new 16-canary
Codex Desktop qualification campaign. The earlier qualified package remains
historical evidence for its own identity and is not carried forward.

## Current checkpoint

- The current Desktop-qualified Plugin version is
  `0.3.0-dev+codex.20260814091408`. Its runtime manifest records
  `source_commit` `b950ac78112bc9570e58c0723b7dcd56c22b067c` and
  `source_clean: true`; its Darwin/arm64 Core identity is
  `0.3.0-dev+sha256:73cb39fcbac178a313ed9e18ef4a0b45e87db70c726e95962c46d43e9c485ac4`.
  All four packaged targets reproduced byte-for-byte across two independent
  builds. The installed cache is byte-for-byte identical to the reviewed
  Plugin source; the official Plugin validator, all four Skill validators,
  installed `version`, `schema get`, and read-only project status pass.
  Independent fresh tasks after Desktop restart loaded this exact installed
  Skill locator and reported the same Plugin/Core identity without changing the
  source Workspace's disabled mode.
- The previous ECP marketplace/provider entries were removed. At qualification
  time, the current package was the sole enabled installed `ecp-codex` provider;
  its installed tree matched the packaged source and every scored task used its
  installed launcher rather than a repository, temporary, ambient, or remembered
  binary.
- The formal immutable campaign is
  `<durable-results-root>/ecp-codex-20260814-091408-r3`; the private local root is
  intentionally not recorded in this public repository.
  The official validator reports: `QUALIFIED: 16 qualification cases passed;
  0 INVALID attempts and 0 extended diagnostics preserved`.
- All 16 qualification canaries ran serially in distinct fresh Codex Desktop
  tasks, each with a unique opaque Git Workspace and dedicated authority. The
  campaign covered direct, indirect, negative, incomplete, multi-turn,
  disabled, blocked, enable, disable, governed-change, and forbidden-external-
  action paths. The final governed case completed one bounded Change, one
  offline GateRun, one PASS Evidence, and Change completion while preserving
  the prohibition on commit, push, deploy, and CI mutation.
- A separate disposable installed-upgrade lifecycle moved one enabled Workspace
  with dedicated authority from the earlier qualified package to the current
  package. Workspace mode, ACTIVE/terminal Change history, GateRun history, and
  Evidence remained queryable; the new Core rejected the old execution-bound
  Evidence as stale, required a new plan and GateRun, then returned PASS and
  completed the Change. No real project, default authority, or ECP source
  Workspace was used for that lifecycle.
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
  campaign. A preflight-only default-authority sentinel mismatch was likewise
  preserved rather than scored; the validator now uses the fixture builder's
  deterministic nonempty tree contract, including regular files and symlinks,
  and the successful campaign started from a new immutable results root.
- Exploratory use in one established real project completed multiple bounded
  local Changes and demonstrated that enabled routing, authority history,
  Gate/Evidence/Verdict, cancellation, and completion work outside synthetic
  fixtures. It also exposed actionable friction: an affected accepted Truth
  Unknown could remain stale after a related PASS Change; adapters could guess
  unsupported enum/payload fields or omit required semantic categories;
  routine full-history reads grew rapidly; conservative impact could trigger
  broad risk/confirmation and full-build cost. This use was on the canonical
  project Workspace without the pilot preregistration, safe-copy controls,
  frozen thresholds, or seven-day observation window, so it is exploratory
  product evidence, not a completed Phase 1B pilot.
- The post-qualification hardening source is now Core `0.3.1-dev`, Plugin
  `0.3.1-dev+codex.20260817115910`, and Change contract version 4. It retains
  version 3 accepted-Unknown dispositions and adds bounded `new_path_roots`,
  Gate-lease-first/no-replace authority export, strict acknowledgement replay,
  and an isolated vendored Schema v3 runtime build contract. Historical
  contract versions 0/2/3 remain readable. The immediately preceding
  `0.3.1-dev+codex.20260817112321` package passed
  `git diff --check`, `go vet ./...`, all 27 Python qualification-protocol
  tests, official Plugin/four-Skill validators, unskipped
  `go test ./... -count=1`, and `go test -race ./... -count=1`; the 84-scenario
  traceability count is exact. It was formally packaged from clean source
  commit `4c1ca18676ded242221605a67233a07b492e34ee`, reproduced byte-for-byte
  from a separate clone, installed, and accepted by a new read-only ephemeral
  task with Darwin/arm64 Core identity
  `0.3.1-dev+sha256:f78828f3d2b9614c7c21211194fc84cef44a8a6b2c5cbf5ea13fa979a5f317df`.
  That fresh task exposed two shared-reference omissions (the exact
  Requirement example and export lock/no-replace description); they are fixed
  in the new cachebuster named above. The new Plugin identity is not yet
  repackaged or installed at this checkpoint. The earlier 16-case campaign
  remains evidence only for its exact older
  Plugin/Core identity. No existing real project's mode, authority history,
  source, or `.ecp` was rewritten by this source work.

## Next actions, in order

1. Run the separately owned, preregistered greenfield and established safe-copy
   pilot tracks without using ECP's source Workspace, a production Workspace, a
   release branch, or a project's only copy.
2. Complete the fixed seven-day observation window and Owner review, then decide
   whether to expand, simplify and retry, or stop from the frozen
   value-versus-friction thresholds.
3. Treat uninstall, remove-then-reinstall, forward rollback, cross-machine/team
   handoff, signing/notarization, public distribution, and release as separate
   explicitly authorized evidence gates; qualification and pilot permission do
   not complete any of them.

The pilot protocol is [docs/real-project-pilot.md](docs/real-project-pilot.md),
the roadmap is [docs/roadmap.md](docs/roadmap.md), and package boundaries are
[docs/distribution.md](docs/distribution.md).
